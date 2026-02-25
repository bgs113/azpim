# Test Coverage Design — azpim

**Date:** 2026-02-25
**Scope:** Pure functions only (no Azure SDK HTTP mocking, no real credentials required)
**Structure:** Mirror-file convention — one test file per source file, extending existing files where tests are missing

---

## Context

The `azpim` tool wraps Azure PIM (Privileged Identity Management) REST APIs via the ARM SDK. Most business logic is pure (string manipulation, duration parsing, deduplication) and fully testable without network access. The existing tests cover:

- `parseDurationInput`, `matchEligible` (cmd)
- `durationToISO8601`, `parseISO8601Duration` (pim/request, pim/policy)
- `deduplicateEligible`, `FilterEligibleActive` (pim/eligible)
- `extractJSONStringValue` (auth)

## Conventions

- Standard `testing` package, table-driven `tests := []struct{...}` format
- No new dependencies
- Run with `go test ./...` from `azpim/`

---

## Files and Functions

### `internal/pim/client_test.go` (new)

**`BuildScope`** — 7 cases:
- Explicit `--scope` passthrough
- Management group by ID
- Tenant root (`"/"`) with tenant ID → resolves to tenant GUID
- Tenant root (`"/"`) without tenant ID → error
- Subscription only
- Subscription + resource group
- No flags set → error

**`PtrString`** — nil pointer → `""`, non-nil → value

### `internal/pim/active_test.go` (new)

**`deduplicateActive`** — mirrors `deduplicateEligible` tests:
- Empty input
- No duplicates unchanged
- Exact duplicate collapsed
- Group wins over Direct for same role+scope
- Group wins regardless of input order
- Same role at different scopes not deduped

**`formatHMS`** — fixed durations → `"HH:MM:SS"` strings

**`formatHuman`** — hours-only, minutes+seconds, seconds-only branches

**`TimeRemaining`** — boundary cases only (avoids flakiness from `time.Until`):
- `HasExpiry=false` → `"Permanent"`
- `EndTime` in the past → `"Expired"`
- `EndTime` 2 hours in the future → non-empty, non-error string (spot-check format)

### `internal/pim/scopes_test.go` (new)

**`lastPathSegment`** — various ARM paths, trailing slash, root, empty string

### Extend `internal/pim/eligible_test.go`

**`roleDefGUID`** — full ARM role def ID → bare GUID, no-slash passthrough

**`normalizeScope`** — uppercase → lowercase, trailing slashes stripped, combined

**`resourceTypeFromScope`** — management group, subscription, resource group, resource (with provider), unknown

### Extend `internal/pim/policy_test.go`

**`FormatDuration`** — whole hours (`"8h"`), hours+minutes (`"1h30m"`), sub-hour (`"0h30m"`)

### `internal/output/table_test.go` (new)

**`truncate`** — short (unchanged), exact length, over length (ellipsis), multi-byte runes

**`formatTime`** — zero time → `"-"`, specific UTC time → expected local format

**`PrintEligibleTable`** — empty slice → `"No eligible assignments found."`, one row → output contains role name

**`PrintActiveTable`** — empty slice → `"No active assignments found."`, one row → output contains role name

### `internal/output/json_test.go` (new)

**`PrintEligibleJSON`** — marshal to buffer, unmarshal to `[]map[string]interface{}`, assert `role_name`, `scope`, `has_expiry`, `end_time` fields

**`PrintActiveJSON`** — same, assert `state`, `time_remaining_display`, `time_remaining_seconds`

### Extend `cmd/activate_test.go`

**`resolveDuration`** (non-interactive path only):
- Flag within max → returns parsed duration
- Flag exceeds max → error
- Flag zero or negative → error
- No max (maxDur=0) → any positive duration accepted

**`selectEligible`** (non-interactive paths only):
- Single exact match → returns it
- No match → error with role name in message

**`printActivateJSON`** — write to buffer, unmarshal, assert `role_name`, `duration`, `duration_seconds`, `activated_at` and `expires_at` differ by duration

---

## Edge Case Strategy

- `TimeRemaining` active branch: test `formatHuman`/`formatHMS` directly with fixed durations rather than controlling `time.Now()`
- Table output assertions: use `strings.Contains` on captured buffer rather than exact format matching
- JSON assertions: `encoding/json.Unmarshal` into `map[string]interface{}` to avoid struct coupling
