# Performance Design: Role Name Caching and Client Reuse

## Problem

`azpim eligible`, `azpim active`, and `azpim activate` are consistently slow even
after the scope list cache is warm (5-minute TTL on `~/.cache/azpim/scopes.json`).

### Root causes

1. **`roleDefCache` is in-memory only.** Every process invocation re-fetches all role
   definition display names from the ARM `RoleDefinitions.Get` API. With 20+ subscriptions
   and multiple eligible assignments per subscription, each run issues many sequential ARM
   calls (one per unique role per goroutine).

2. **No singleflight deduplication.** `ResolveRoleName` uses a check-unlock-fetch-lock-write
   pattern. Multiple goroutines that all miss the cache before any one of them completes the
   API call will all independently issue the same ARM call, wasting round-trips.

3. **`fetchScopeName` and the scope-enumeration helpers create new SDK client instances on
   every call.** `armsubscriptions.NewClient` and `armmanagementgroups.NewClient` are called
   inline in `fetchScopeName`, `listSubscriptionEntries`, `listManagementGroupEntries`, and
   `ResolveSubscriptionID` instead of reusing shared instances.

## Solution

Three targeted changes, in priority order:

### 1. Disk-cache role definition names

Persist `roleDefCache` to `~/.cache/azpim/roledefs.json` (same directory as
`scopes.json`) with a 24-hour TTL. Role display names essentially never change, so
a 24h TTL is conservative.

After the first run, all `ResolveRoleName` calls become instant in-memory lookups
pre-populated from disk — eliminating the dominant source of latency on warm runs.

**Cache schema** (mirrors the scope cache):

```json
{
  "fetched_at": "2026-02-25T12:00:00Z",
  "entries": [
    {
      "role_def_id": "/providers/Microsoft.Authorization/roleDefinitions/<guid>",
      "display_name": "Contributor"
    }
  ]
}
```

**Lifecycle:**

- `NewClients` calls `loadRoleDefDiskCache` to read the file, validate the TTL, and
  pre-populate `roleDefCache`.
- Each `cmd/*.go` `RunE` defers `clients.SaveRoleDefCache()` immediately after a
  successful `NewClients(...)` call. The defer fires whether the command succeeds or
  errors, ensuring the cache is always flushed.
- Cache helpers live in a new `internal/pim/rolecache.go` file.

### 2. Singleflight deduplication for `ResolveRoleName`

Add `golang.org/x/sync/singleflight` as a direct dependency. Add a
`roleDefFlight singleflight.Group` field to `Clients`.

Update `ResolveRoleName`:

```
check roleDefCache → return if hit (fast path, unchanged)

roleDefFlight.Do(roleDefID, func() {
    call RoleDefinitions.Get(ctx, scope, guid)
    write result to roleDefCache under mu
    return name
})
```

If multiple goroutines concurrently miss the cache for the same `roleDefID`, only
one ARM call is issued; all waiters receive the result when it completes. The group
resets after each call so future invocations are unaffected.

### 3. Reuse subscription and management group clients

Add `Subscriptions *armsubscriptions.Client` and `ManagementGroups *armmanagementgroups.Client`
to `Clients` and initialize them in `NewClients`.

Update four call sites to use the stored fields instead of creating clients inline:

- `fetchScopeName` (two branches: MG and subscription)
- `listSubscriptionEntries`
- `listManagementGroupEntries`
- `ResolveSubscriptionID`

This removes per-call instantiation overhead and ensures HTTP connections are pooled
across all calls in a session.

## Files changed

| File | Change |
|------|--------|
| `internal/pim/rolecache.go` | New: disk cache helpers (`loadRoleDefDiskCache`, `saveRoleDefDiskCache`, path/TTL constants) |
| `internal/pim/client.go` | Add `Subscriptions`, `ManagementGroups`, `roleDefFlight` fields; call `loadRoleDefDiskCache` in `NewClients`; add `SaveRoleDefCache()` method |
| `internal/pim/scopes.go` | Use stored `c.Subscriptions`/`c.ManagementGroups` in all four call sites |
| `cmd/eligible.go` | Defer `clients.SaveRoleDefCache()` |
| `cmd/active.go` | Defer `clients.SaveRoleDefCache()` |
| `cmd/activate.go` | Defer `clients.SaveRoleDefCache()` |
| `cmd/deactivate.go` | Defer `clients.SaveRoleDefCache()` |
| `cmd/extend.go` | Defer `clients.SaveRoleDefCache()` |
| `internal/pim/rolecache_test.go` | New: unit tests for disk cache load/save round-trip using temp dirs |
| `internal/pim/client_test.go` | Extend: `TestResolveRoleNameSingleFlight` (stubbed client, assert single API call) |
| `go.mod` / `go.sum` | Add `golang.org/x/sync` direct dependency |

## Expected outcome

| Scenario | Before | After |
|----------|--------|-------|
| First-ever run (cold cache) | Slow (N serial role API calls per goroutine) | Slightly faster (singleflight deduplicates concurrent goroutines) |
| Any subsequent run (warm role cache) | Slow (N serial role API calls per goroutine) | Fast (all role lookups are instant map reads from disk cache) |
| `fetchScopeName` for non-subscription scopes | New client per call | Reuses pooled client |

## Non-goals

- Caching active/eligible assignment results (would require invalidation logic and add
  complexity disproportionate to benefit)
- Scope name disk cache beyond subscriptions/MGs (already pre-populated from scope list)
- In-goroutine parallelization of role name lookups (redundant with disk cache + singleflight)
