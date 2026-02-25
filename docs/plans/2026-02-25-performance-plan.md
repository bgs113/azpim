# Performance: Role Name Caching and Client Reuse — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate per-invocation ARM API calls for role definition name lookups by persisting the in-memory `roleDefCache` to disk, and reduce duplicate concurrent lookups via `singleflight`.

**Architecture:** Add `~/.cache/azpim/roledefs.json` (24h TTL) loaded at `NewClients` startup and flushed at command exit via `defer clients.SaveRoleDefCache()`. Add `roleDefGetter` interface to enable test injection. Add `singleflight.Group` to `Clients` to coalesce concurrent misses. Move shared subscription/MG client construction from per-call site to `NewClients`.

**Tech Stack:** `golang.org/x/sync/singleflight`, standard `encoding/json`/`os`/`sync`.

---

### Task 1: Add `golang.org/x/sync` dependency

**Files:**
- Modify: `go.mod`, `go.sum`

**Step 1: Add the dependency**

```bash
cd /path/to/azpim
go get golang.org/x/sync@latest
```

Expected: `go.mod` gains a `require golang.org/x/sync vX.Y.Z` line; `go.sum` updated.

**Step 2: Verify the module resolves**

```bash
go mod tidy
go build ./...
```

Expected: clean build, no errors.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add golang.org/x/sync dependency for singleflight"
```

---

### Task 2: Role definition disk cache helpers (TDD)

**Files:**
- Create: `internal/pim/rolecache_test.go`
- Create: `internal/pim/rolecache.go`

**Step 1: Write the failing tests**

Create `internal/pim/rolecache_test.go`:

```go
package pim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoleDefDiskCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roledefs.json")

	want := map[string]string{
		"/providers/Microsoft.Authorization/roleDefinitions/abc": "Contributor",
		"/providers/Microsoft.Authorization/roleDefinitions/def": "Owner",
	}

	saveRoleDefDiskCache(path, want)

	got := loadRoleDefDiskCache(path)
	if got == nil {
		t.Fatal("loadRoleDefDiskCache returned nil")
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestRoleDefDiskCacheExpired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roledefs.json")

	// Write a cache file with an old timestamp.
	stale := roleDefCacheFile{
		FetchedAt: time.Now().Add(-25 * time.Hour),
		Entries:   []roleDefEntryJSON{{RoleDefID: "/providers/foo", DisplayName: "Foo"}},
	}
	data, _ := json.Marshal(stale)
	_ = os.WriteFile(path, data, 0600)

	got := loadRoleDefDiskCache(path)
	if got != nil {
		t.Errorf("expected nil for expired cache, got %v", got)
	}
}

func TestRoleDefDiskCacheCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roledefs.json")

	_ = os.WriteFile(path, []byte("not valid json {{{"), 0600)

	got := loadRoleDefDiskCache(path)
	if got != nil {
		t.Errorf("expected nil for corrupt cache, got %v", got)
	}
}

func TestRoleDefDiskCacheMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")

	got := loadRoleDefDiskCache(path)
	if got != nil {
		t.Errorf("expected nil for missing file, got %v", got)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/pim/ -run TestRoleDefDiskCache -v
```

Expected: compile error — `saveRoleDefDiskCache`, `loadRoleDefDiskCache`, `roleDefCacheFile`, `roleDefEntryJSON` not defined.

**Step 3: Implement `internal/pim/rolecache.go`**

```go
package pim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const roleDefCacheTTL = 24 * time.Hour

type roleDefCacheFile struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Entries   []roleDefEntryJSON `json:"entries"`
}

type roleDefEntryJSON struct {
	RoleDefID   string `json:"role_def_id"`
	DisplayName string `json:"display_name"`
}

func roleDefCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "azpim", "roledefs.json"), nil
}

// loadRoleDefDiskCache reads role definition names from a cache file.
// Returns nil if the file is missing, corrupt, or older than roleDefCacheTTL.
func loadRoleDefDiskCache(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cf roleDefCacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil
	}
	if time.Since(cf.FetchedAt) > roleDefCacheTTL {
		return nil
	}
	m := make(map[string]string, len(cf.Entries))
	for _, e := range cf.Entries {
		m[e.RoleDefID] = e.DisplayName
	}
	return m
}

// saveRoleDefDiskCache writes role definition names to a cache file.
// Failures are silently ignored.
func saveRoleDefDiskCache(path string, m map[string]string) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	entries := make([]roleDefEntryJSON, 0, len(m))
	for id, name := range m {
		entries = append(entries, roleDefEntryJSON{RoleDefID: id, DisplayName: name})
	}
	data, err := json.Marshal(roleDefCacheFile{FetchedAt: time.Now(), Entries: entries})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/pim/ -run TestRoleDefDiskCache -v
```

Expected: all 4 tests PASS.

**Step 5: Commit**

```bash
git add internal/pim/rolecache.go internal/pim/rolecache_test.go
git commit -m "feat(pim): add role definition name disk cache helpers"
```

---

### Task 3: Add `Subscriptions`/`ManagementGroups` clients to `Clients`, update `scopes.go`

**Files:**
- Modify: `internal/pim/client.go`
- Modify: `internal/pim/scopes.go`

This is a pure refactor — no behavior change. The four sites that currently call
`armsubscriptions.NewClient` or `armmanagementgroups.NewClient` inline are updated
to use shared fields.

**Step 1: Add fields and initialize them in `NewClients`**

In `internal/pim/client.go`, add two fields to the `Clients` struct (after `Policies`):

```go
Subscriptions    *armsubscriptions.Client
ManagementGroups *armmanagementgroups.Client
```

Add the required import at the top:
```go
"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/managementgroups/armmanagementgroups"
"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
```

In `NewClients`, after creating `policies` and before the `return`, add:

```go
subscriptions, err := armsubscriptions.NewClient(cred, &opts)
if err != nil {
    return nil, fmt.Errorf("create subscriptions client: %w", err)
}

managementGroups, err := armmanagementgroups.NewClient(cred, &opts)
if err != nil {
    return nil, fmt.Errorf("create management groups client: %w", err)
}
```

Add them to the returned struct:
```go
Subscriptions:    subscriptions,
ManagementGroups: managementGroups,
```

**Step 2: Update `scopes.go` to use stored clients**

In `internal/pim/scopes.go`:

1. `listSubscriptionEntries` — replace the inline client creation:

   Remove:
   ```go
   client, err := armsubscriptions.NewClient(c.cred, &arm.ClientOptions{})
   if err != nil {
       return nil, err
   }
   pager := client.NewListPager(nil)
   ```
   Replace with:
   ```go
   pager := c.Subscriptions.NewListPager(nil)
   ```

2. `listManagementGroupEntries` — replace inline creation:

   Remove:
   ```go
   client, err := armmanagementgroups.NewClient(c.cred, &arm.ClientOptions{})
   if err != nil {
       return nil, err
   }
   pager := client.NewListPager(nil)
   ```
   Replace with:
   ```go
   pager := c.ManagementGroups.NewListPager(nil)
   ```

3. `fetchScopeName` — replace two inline client creations:

   In the MG branch, remove:
   ```go
   mgClient, err := armmanagementgroups.NewClient(c.cred, &arm.ClientOptions{})
   if err == nil {
       resp, err := mgClient.Get(ctx, mgID, nil)
       ...
   }
   ```
   Replace with:
   ```go
   resp, err := c.ManagementGroups.Get(ctx, mgID, nil)
   if err == nil && resp.Properties != nil && resp.Properties.DisplayName != nil && *resp.Properties.DisplayName != "" {
       return *resp.Properties.DisplayName
   }
   ```

   In the subscription branch, remove:
   ```go
   subClient, err := armsubscriptions.NewClient(c.cred, &arm.ClientOptions{})
   if err == nil {
       resp, err := subClient.Get(ctx, rest, nil)
       ...
   }
   ```
   Replace with:
   ```go
   resp, err := c.Subscriptions.Get(ctx, rest, nil)
   if err == nil && resp.DisplayName != nil && *resp.DisplayName != "" {
       return *resp.DisplayName
   }
   ```

4. `ResolveSubscriptionID` — calls `c.listSubscriptionEntries(ctx)` which now uses the stored client. No direct change needed.

5. Remove the now-unused imports from `scopes.go` (the `arm.ClientOptions` import is no longer needed if no code in scopes.go uses it; check and clean up). The `arm` import (`github.com/Azure/azure-sdk-for-go/sdk/azcore/arm`) is used only for `arm.ClientOptions{}` in the removed lines. Remove it. The `armmanagementgroups` and `armsubscriptions` imports are still used in `listSubscriptionEntries`/`listManagementGroupEntries` for the type assertions on page values (keep them).

**Step 3: Build and run tests**

```bash
go build ./...
go test ./internal/pim/ -v
```

Expected: clean build, all existing tests pass.

**Step 4: Commit**

```bash
git add internal/pim/client.go internal/pim/scopes.go
git commit -m "refactor(pim): store shared subscription/MG clients in Clients struct"
```

---

### Task 4: Extract `roleDefGetter` interface + add singleflight to `ResolveRoleName` (TDD)

**Files:**
- Modify: `internal/pim/client_test.go`
- Modify: `internal/pim/client.go`

**Step 1: Write the failing tests**

Append to `internal/pim/client_test.go`:

```go
import (
    // add to existing imports:
    "context"
    "sync"
    "time"

    "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3"
)

// fakeRoleDefGetter is a test double for the roleDefGetter interface.
type fakeRoleDefGetter struct {
    mu    sync.Mutex
    calls int
    name  string
    delay time.Duration
}

func (f *fakeRoleDefGetter) Get(_ context.Context, _, _ string, _ *armauthorization.RoleDefinitionsClientGetOptions) (armauthorization.RoleDefinitionsClientGetResponse, error) {
    if f.delay > 0 {
        time.Sleep(f.delay)
    }
    f.mu.Lock()
    f.calls++
    f.mu.Unlock()
    props := armauthorization.RoleDefinitionProperties{RoleName: &f.name}
    return armauthorization.RoleDefinitionsClientGetResponse{
        RoleDefinition: armauthorization.RoleDefinition{Properties: &props},
    }, nil
}

func TestResolveRoleNameCachesResult(t *testing.T) {
    name := "Contributor"
    fake := &fakeRoleDefGetter{name: name}
    c := &Clients{
        RoleDefinitions: fake,
        roleDefCache:    make(map[string]string),
    }
    roleDefID := "/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c"

    got := c.ResolveRoleName(context.Background(), roleDefID)
    if got != name {
        t.Errorf("got %q, want %q", got, name)
    }
    if fake.calls != 1 {
        t.Errorf("expected 1 API call on first lookup, got %d", fake.calls)
    }

    got2 := c.ResolveRoleName(context.Background(), roleDefID)
    if got2 != name {
        t.Errorf("second call got %q, want %q", got2, name)
    }
    if fake.calls != 1 {
        t.Errorf("expected still 1 API call after cache hit, got %d (cache not working)", fake.calls)
    }
}

func TestResolveRoleNameSingleFlight(t *testing.T) {
    const N = 20
    name := "Owner"
    fake := &fakeRoleDefGetter{name: name, delay: 20 * time.Millisecond}
    c := &Clients{
        RoleDefinitions: fake,
        roleDefCache:    make(map[string]string),
    }
    roleDefID := "/providers/Microsoft.Authorization/roleDefinitions/8e3af657-a8ff-443c-a75c-2fe8c4bcb635"

    var wg sync.WaitGroup
    results := make([]string, N)
    for i := range N {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            results[i] = c.ResolveRoleName(context.Background(), roleDefID)
        }(i)
    }
    wg.Wait()

    for i, r := range results {
        if r != name {
            t.Errorf("results[%d] = %q, want %q", i, r, name)
        }
    }

    fake.mu.Lock()
    calls := fake.calls
    fake.mu.Unlock()

    // singleflight should coalesce concurrent misses; with 20ms delay and 20
    // goroutines, all should arrive before the first returns → exactly 1 call.
    // We allow up to 3 for CI scheduling jitter.
    if calls > 3 {
        t.Errorf("expected ≤3 API calls (singleflight), got %d for %d goroutines", calls, N)
    }
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/pim/ -run "TestResolveRoleName" -v
```

Expected: compile error — `fakeRoleDefGetter` does not implement `*armauthorization.RoleDefinitionsClient` (field type mismatch) until the interface is extracted.

**Step 3: Extract `roleDefGetter` interface and change `Clients.RoleDefinitions` field type**

In `internal/pim/client.go`, add the interface definition just before the `Clients` struct:

```go
// roleDefGetter is the subset of [*armauthorization.RoleDefinitionsClient] used
// for role name resolution. Extracted as an interface to allow test injection.
type roleDefGetter interface {
    Get(ctx context.Context, scope string, roleDefinitionID string, options *armauthorization.RoleDefinitionsClientGetOptions) (armauthorization.RoleDefinitionsClientGetResponse, error)
}
```

Change the `RoleDefinitions` field in the `Clients` struct from:
```go
RoleDefinitions   *armauthorization.RoleDefinitionsClient
```
to:
```go
RoleDefinitions roleDefGetter
```

Add the `roleDefFlight` field to `Clients`:
```go
import "golang.org/x/sync/singleflight"

// in Clients struct:
roleDefFlight singleflight.Group
```

The `NewClients` function already assigns `RoleDefinitions: roleDefs` where `roleDefs` is `*armauthorization.RoleDefinitionsClient` — that concrete type satisfies the interface, so no change is needed there.

**Step 4: Run tests — `TestResolveRoleNameCachesResult` should pass, `TestResolveRoleNameSingleFlight` should fail**

```bash
go test ./internal/pim/ -run "TestResolveRoleName" -v
```

Expected:
- `TestResolveRoleNameCachesResult` PASS (existing cache logic already works)
- `TestResolveRoleNameSingleFlight` FAIL — 20 API calls instead of ≤3

**Step 5: Update `ResolveRoleName` to use singleflight**

In `internal/pim/client.go`, replace the body of `ResolveRoleName` with:

```go
func (c *Clients) ResolveRoleName(ctx context.Context, roleDefID string) string {
    c.mu.Lock()
    if name, ok := c.roleDefCache[roleDefID]; ok {
        c.mu.Unlock()
        return name
    }
    c.mu.Unlock()

    parts := strings.Split(roleDefID, "/providers/Microsoft.Authorization/roleDefinitions/")
    if len(parts) != 2 {
        return roleDefID
    }
    scope := parts[0]
    if scope == "" {
        scope = "/"
    }
    guid := parts[1]

    val, _, _ := c.roleDefFlight.Do(roleDefID, func() (any, error) {
        resp, err := c.RoleDefinitions.Get(ctx, scope, guid, nil)
        name := guid // fallback to GUID on error
        if err == nil && resp.Properties != nil && resp.Properties.RoleName != nil && *resp.Properties.RoleName != "" {
            name = *resp.Properties.RoleName
        }
        c.mu.Lock()
        c.roleDefCache[roleDefID] = name
        c.mu.Unlock()
        return name, nil
    })
    return val.(string)
}
```

**Step 6: Run tests — both should pass**

```bash
go test ./internal/pim/ -run "TestResolveRoleName" -v
go test ./internal/pim/ -v
```

Expected: all tests PASS.

**Step 7: Commit**

```bash
git add internal/pim/client.go internal/pim/client_test.go
git commit -m "feat(pim): add singleflight deduplication to ResolveRoleName"
```

---

### Task 5: Wire disk cache into `NewClients` + add `SaveRoleDefCache()` (TDD)

**Files:**
- Modify: `internal/pim/client_test.go`
- Modify: `internal/pim/client.go`

**Step 1: Write the failing test**

Append to `internal/pim/client_test.go`:

```go
func TestSaveRoleDefCache(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "roledefs.json")

    c := &Clients{
        roleDefCache: map[string]string{
            "/providers/Microsoft.Authorization/roleDefinitions/abc": "Contributor",
        },
    }

    c.saveRoleDefCacheTo(path)

    got := loadRoleDefDiskCache(path)
    if got == nil {
        t.Fatal("expected cache to be saved, got nil")
    }
    if got["/providers/Microsoft.Authorization/roleDefinitions/abc"] != "Contributor" {
        t.Errorf("unexpected cache content: %v", got)
    }
}
```

Note: this test uses a helper `c.saveRoleDefCacheTo(path)` which we'll add alongside `SaveRoleDefCache()`. The public method uses the real path; the helper takes a path for testability.

Also add the import `"path/filepath"` to `client_test.go` if not already present.

**Step 2: Run to verify it fails**

```bash
go test ./internal/pim/ -run TestSaveRoleDefCache -v
```

Expected: compile error — `saveRoleDefCacheTo` not defined.

**Step 3: Add `SaveRoleDefCache` and `saveRoleDefCacheTo` to `client.go`**

```go
// saveRoleDefCacheTo flushes the current in-memory role definition name cache
// to the given path. Used internally and by tests.
func (c *Clients) saveRoleDefCacheTo(path string) {
    c.mu.Lock()
    m := make(map[string]string, len(c.roleDefCache))
    for k, v := range c.roleDefCache {
        m[k] = v
    }
    c.mu.Unlock()
    saveRoleDefDiskCache(path, m)
}

// SaveRoleDefCache flushes the in-memory role definition name cache to disk.
// Call via defer immediately after NewClients succeeds.
func (c *Clients) SaveRoleDefCache() {
    path, err := roleDefCachePath()
    if err != nil {
        return
    }
    c.saveRoleDefCacheTo(path)
}
```

**Step 4: Load the disk cache in `NewClients`**

In `NewClients`, find where `roleDefCache` is initialized:

```go
roleDefCache: make(map[string]string),
```

Replace with:

```go
roleDefCache: func() map[string]string {
    if path, err := roleDefCachePath(); err == nil {
        if loaded := loadRoleDefDiskCache(path); loaded != nil {
            return loaded
        }
    }
    return make(map[string]string)
}(),
```

**Step 5: Run tests**

```bash
go test ./internal/pim/ -v
```

Expected: all tests PASS including `TestSaveRoleDefCache`.

**Step 6: Commit**

```bash
git add internal/pim/client.go internal/pim/client_test.go
git commit -m "feat(pim): load/save role definition name disk cache in Clients"
```

---

### Task 6: Defer `SaveRoleDefCache` in all command `RunE` functions

**Files:**
- Modify: `cmd/eligible.go`
- Modify: `cmd/active.go`
- Modify: `cmd/activate.go`
- Modify: `cmd/deactivate.go`
- Modify: `cmd/extend.go`

**Step 1: Update each command**

In each file, find the block that creates `clients` and immediately add the defer on the line after the error check. The pattern is identical across all 5 files.

**`cmd/eligible.go`** — after line 40 (`return err`):
```go
clients, err := pim.NewClients(cred)
if err != nil {
    return err
}
defer clients.SaveRoleDefCache()  // ← add this line
```

**`cmd/active.go`** — after line 42 (`return err`):
```go
clients, err := pim.NewClients(cred)
if err != nil {
    return err
}
defer clients.SaveRoleDefCache()  // ← add this line
```

**`cmd/activate.go`** — after line 52 (`return err`):
```go
clients, err := pim.NewClients(cred)
if err != nil {
    return err
}
defer clients.SaveRoleDefCache()  // ← add this line
```

**`cmd/deactivate.go`** — after line 46 (`return err`):
```go
clients, err := pim.NewClients(cred)
if err != nil {
    return err
}
defer clients.SaveRoleDefCache()  // ← add this line
```

**`cmd/extend.go`** — after line 49 (`return err`):
```go
clients, err := pim.NewClients(cred)
if err != nil {
    return err
}
defer clients.SaveRoleDefCache()  // ← add this line
```

**Step 2: Build**

```bash
go build ./...
```

Expected: clean build, no errors.

**Step 3: Run full test suite**

```bash
go test ./...
```

Expected: all tests PASS.

**Step 4: Commit**

```bash
git add cmd/eligible.go cmd/active.go cmd/activate.go cmd/deactivate.go cmd/extend.go
git commit -m "feat(cmd): persist role definition name cache on command exit"
```

---

### Task 7: Final verification

**Step 1: Run tests with race detector**

```bash
go test -race ./...
```

Expected: all tests PASS with no race conditions detected.

**Step 2: Verify test coverage (informational)**

```bash
go test -coverprofile=cover.out ./...
go tool cover -func=cover.out | grep -E "(pim|output|cmd)"
```

Note the `internal/pim` coverage — should be higher than the pre-task baseline of 28.9%.

**Step 3: Verify the binary builds for the target platform**

```bash
go build -o /dev/null ./cmd/azpim/
```

(Adjust the main package path if different — check `go build ./...` output.)

**Step 4: Commit any final cleanup**

If any formatting or minor cleanup is needed:

```bash
gofmt -w ./internal/pim/ ./cmd/
go test -race ./...
git add -p
git commit -m "chore: gofmt and final cleanup"
```
