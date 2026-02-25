# Test Coverage Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add unit tests for all untested pure functions across `internal/pim`, `internal/output`, and `cmd` packages.

**Architecture:** Mirror-file convention — one test file per source file, extending existing test files where functions were missed. All tests use standard `testing` package with table-driven patterns. Since the implementations already exist, tests should pass immediately (unlike traditional TDD where you write a failing test first).

**Tech Stack:** Go standard library `testing`, `bytes`, `encoding/json`, `strings`, `time`. No new dependencies.

---

## Task 1: `internal/pim/client_test.go` — BuildScope, PtrString

**Files:**
- Create: `azpim/internal/pim/client_test.go`

**Step 1: Create the test file**

```go
package pim

import "testing"

func TestBuildScope(t *testing.T) {
	tests := []struct {
		managementGroup string
		tenantID        string
		subscription    string
		resourceGroup   string
		explicitScope   string
		want            string
		wantErr         bool
	}{
		// explicit scope takes priority over everything else
		{"", "", "", "", "/subscriptions/abc", "/subscriptions/abc", false},
		// management group by ID
		{"mg-1", "", "", "", "", "/providers/Microsoft.Management/managementGroups/mg-1", false},
		// tenant root ("/") requires tenantID
		{"/", "tenant-123", "", "", "", "/providers/Microsoft.Management/managementGroups/tenant-123", false},
		// tenant root without tenantID → error
		{"/", "", "", "", "", "", true},
		// subscription only
		{"", "", "sub-1", "", "", "/subscriptions/sub-1", false},
		// subscription + resource group
		{"", "", "sub-1", "rg-1", "", "/subscriptions/sub-1/resourceGroups/rg-1", false},
		// nothing set → error
		{"", "", "", "", "", "", true},
	}
	for _, tt := range tests {
		got, err := BuildScope(tt.managementGroup, tt.tenantID, tt.subscription, tt.resourceGroup, tt.explicitScope)
		if (err != nil) != tt.wantErr {
			t.Errorf("BuildScope(%q,%q,%q,%q,%q): err=%v, wantErr=%v",
				tt.managementGroup, tt.tenantID, tt.subscription, tt.resourceGroup, tt.explicitScope, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("BuildScope(...) = %q, want %q", got, tt.want)
		}
	}
}

func TestPtrString(t *testing.T) {
	if got := PtrString(nil); got != "" {
		t.Errorf("PtrString(nil) = %q, want \"\"", got)
	}
	s := "hello"
	if got := PtrString(&s); got != "hello" {
		t.Errorf("PtrString(%q) = %q, want %q", s, got, s)
	}
}
```

**Step 2: Run tests to verify they pass**

```bash
cd azpim && go test ./internal/pim/ -run "TestBuildScope|TestPtrString" -v
```

Expected: both PASS

**Step 3: Commit**

```bash
git add internal/pim/client_test.go
git commit -m "test: add BuildScope and PtrString tests"
```

---

## Task 2: `internal/pim/active_test.go` — deduplicateActive, formatHMS, formatHuman, TimeRemaining

**Files:**
- Create: `azpim/internal/pim/active_test.go`

Note: `eligible_test.go` already defines `newActive(guid, scope, membership string) ActiveAssignment` at package level — reuse it here.

**Step 1: Create the test file**

```go
package pim

import (
	"testing"
	"time"
)

func TestDeduplicateActive(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		if got := deduplicateActive(nil); len(got) != 0 {
			t.Errorf("got %d, want 0", len(got))
		}
	})

	t.Run("no duplicates unchanged", func(t *testing.T) {
		in := []ActiveAssignment{
			newActive("guid1", "/subscriptions/sub1", "Direct"),
			newActive("guid2", "/subscriptions/sub1", "Direct"),
		}
		if got := deduplicateActive(in); len(got) != 2 {
			t.Errorf("got %d, want 2", len(got))
		}
	})

	t.Run("exact duplicate collapsed", func(t *testing.T) {
		a := newActive("guid1", "/subscriptions/sub1", "Direct")
		if got := deduplicateActive([]ActiveAssignment{a, a}); len(got) != 1 {
			t.Errorf("got %d, want 1", len(got))
		}
	})

	t.Run("group wins over direct for same role+scope", func(t *testing.T) {
		direct := newActive("guid1", "/subscriptions/sub1", "Direct")
		group := newActive("guid1", "/subscriptions/sub1", "Group")
		got := deduplicateActive([]ActiveAssignment{direct, group})
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1", len(got))
		}
		if got[0].MembershipType != "Group" {
			t.Errorf("membership = %q, want Group", got[0].MembershipType)
		}
	})

	t.Run("group wins regardless of input order", func(t *testing.T) {
		direct := newActive("guid1", "/subscriptions/sub1", "Direct")
		group := newActive("guid1", "/subscriptions/sub1", "Group")
		got := deduplicateActive([]ActiveAssignment{group, direct})
		if len(got) != 1 || got[0].MembershipType != "Group" {
			t.Errorf("got %v, want single Group entry", got)
		}
	})

	t.Run("same role at different scopes not deduped", func(t *testing.T) {
		a := newActive("guid1", "/subscriptions/sub1", "Direct")
		b := newActive("guid1", "/subscriptions/sub2", "Direct")
		if got := deduplicateActive([]ActiveAssignment{a, b}); len(got) != 2 {
			t.Errorf("got %d, want 2", len(got))
		}
	})
}

func TestFormatHMS(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{4*time.Hour + 30*time.Minute + 15*time.Second, "04:30:15"},
		{1*time.Hour + 2*time.Minute + 3*time.Second, "01:02:03"},
		{30 * time.Second, "00:00:30"},
		{0, "00:00:00"},
	}
	for _, tt := range tests {
		got := formatHMS(tt.d)
		if got != tt.want {
			t.Errorf("formatHMS(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatHuman(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{4*time.Hour + 30*time.Minute + 15*time.Second, "4h 30m 15s"},
		{2 * time.Hour, "2h 0m 0s"},
		{45*time.Minute + 5*time.Second, "45m 5s"},
		{30 * time.Second, "30s"},
	}
	for _, tt := range tests {
		got := formatHuman(tt.d)
		if got != tt.want {
			t.Errorf("formatHuman(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestTimeRemaining(t *testing.T) {
	t.Run("no expiry returns Permanent", func(t *testing.T) {
		a := &ActiveAssignment{HasExpiry: false}
		if got := a.TimeRemaining(false); got != "Permanent" {
			t.Errorf("TimeRemaining(false) = %q, want Permanent", got)
		}
		if got := a.TimeRemaining(true); got != "Permanent" {
			t.Errorf("TimeRemaining(true) = %q, want Permanent", got)
		}
	})

	t.Run("expired end time returns Expired", func(t *testing.T) {
		a := &ActiveAssignment{
			HasExpiry: true,
			EndTime:   time.Now().Add(-1 * time.Hour),
		}
		if got := a.TimeRemaining(false); got != "Expired" {
			t.Errorf("TimeRemaining = %q, want Expired", got)
		}
	})

	t.Run("future end time returns non-empty string", func(t *testing.T) {
		a := &ActiveAssignment{
			HasExpiry: true,
			EndTime:   time.Now().Add(2 * time.Hour),
		}
		for _, humanReadable := range []bool{false, true} {
			got := a.TimeRemaining(humanReadable)
			if got == "" || got == "Expired" || got == "Permanent" {
				t.Errorf("TimeRemaining(humanReadable=%v) = %q, unexpected value", humanReadable, got)
			}
		}
	})
}
```

**Step 2: Run tests to verify they pass**

```bash
cd azpim && go test ./internal/pim/ -run "TestDeduplicateActive|TestFormatHMS|TestFormatHuman|TestTimeRemaining" -v
```

Expected: all PASS

**Step 3: Commit**

```bash
git add internal/pim/active_test.go
git commit -m "test: add deduplicateActive, formatHMS, formatHuman, TimeRemaining tests"
```

---

## Task 3: `internal/pim/scopes_test.go` — lastPathSegment

**Files:**
- Create: `azpim/internal/pim/scopes_test.go`

**Step 1: Create the test file**

```go
package pim

import "testing"

func TestLastPathSegment(t *testing.T) {
	tests := []struct {
		scope string
		want  string
	}{
		{"/subscriptions/sub1", "sub1"},
		{"/subscriptions/sub1/resourceGroups/rg1", "rg1"},
		{"/providers/Microsoft.Management/managementGroups/mg1", "mg1"},
		{"/subscriptions/sub1/", "sub1"}, // trailing slash stripped
		{"noSlash", "noSlash"},
		{"", ""},
	}
	for _, tt := range tests {
		got := lastPathSegment(tt.scope)
		if got != tt.want {
			t.Errorf("lastPathSegment(%q) = %q, want %q", tt.scope, got, tt.want)
		}
	}
}
```

**Step 2: Run tests to verify they pass**

```bash
cd azpim && go test ./internal/pim/ -run "TestLastPathSegment" -v
```

Expected: PASS

**Step 3: Commit**

```bash
git add internal/pim/scopes_test.go
git commit -m "test: add lastPathSegment tests"
```

---

## Task 4: Extend `internal/pim/eligible_test.go` — roleDefGUID, normalizeScope, resourceTypeFromScope

**Files:**
- Modify: `azpim/internal/pim/eligible_test.go` (append at end of file)

**Step 1: Append these test functions to the end of the file**

```go
func TestRoleDefGUID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/subscriptions/sub1/providers/Microsoft.Authorization/roleDefinitions/abc-123", "abc-123"},
		{"/providers/Microsoft.Authorization/roleDefinitions/abc-123", "abc-123"},
		{"abc-123", "abc-123"}, // bare GUID — no slash, returned as-is
		{"", ""},
	}
	for _, tt := range tests {
		got := roleDefGUID(tt.input)
		if got != tt.want {
			t.Errorf("roleDefGUID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeScope(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/subscriptions/Sub1", "/subscriptions/sub1"},
		{"/subscriptions/sub1/", "/subscriptions/sub1"},
		{"/SUBSCRIPTIONS/SUB1/", "/subscriptions/sub1"},
		{"/subscriptions/sub1", "/subscriptions/sub1"}, // already normalized
	}
	for _, tt := range tests {
		got := normalizeScope(tt.input)
		if got != tt.want {
			t.Errorf("normalizeScope(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResourceTypeFromScope(t *testing.T) {
	tests := []struct {
		scope string
		want  string
	}{
		{"/providers/Microsoft.Management/managementGroups/mg1", "Management group"},
		{"/subscriptions/sub1", "Subscription"},
		{"/subscriptions/sub1/resourceGroups/rg1", "Resource group"},
		// resource group + provider path → "Resource" (provider wins)
		{"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/sa1", "Resource"},
		// provider path not a management group → "Resource"
		{"/providers/Microsoft.Authorization/roleDefinitions/abc", "Resource"},
		{"", ""},
	}
	for _, tt := range tests {
		got := resourceTypeFromScope(tt.scope)
		if got != tt.want {
			t.Errorf("resourceTypeFromScope(%q) = %q, want %q", tt.scope, got, tt.want)
		}
	}
}
```

**Step 2: Run tests to verify they pass**

```bash
cd azpim && go test ./internal/pim/ -run "TestRoleDefGUID|TestNormalizeScope|TestResourceTypeFromScope" -v
```

Expected: all PASS

**Step 3: Commit**

```bash
git add internal/pim/eligible_test.go
git commit -m "test: add roleDefGUID, normalizeScope, resourceTypeFromScope tests"
```

---

## Task 5: Extend `internal/pim/policy_test.go` — FormatDuration

**Files:**
- Modify: `azpim/internal/pim/policy_test.go` (append at end of file)

**Step 1: Append this test function to the end of the file**

```go
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{8 * time.Hour, "8h"},
		{1 * time.Hour, "1h"},
		{time.Hour + 30*time.Minute, "1h30m"},
		{4*time.Hour + 30*time.Minute, "4h30m"},
		{30 * time.Minute, "0h30m"}, // sub-hour: h=0 so produces "0h30m"
	}
	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
```

**Step 2: Run tests to verify they pass**

```bash
cd azpim && go test ./internal/pim/ -run "TestFormatDuration" -v
```

Expected: PASS

**Step 3: Commit**

```bash
git add internal/pim/policy_test.go
git commit -m "test: add FormatDuration tests"
```

---

## Task 6: `internal/output/table_test.go` — truncate, formatTime, PrintEligibleTable, PrintActiveTable

**Files:**
- Create: `azpim/internal/output/table_test.go`

**Step 1: Create the test file**

```go
package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"azpim/internal/pim"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"},    // shorter than limit
		{"hello", 5, "hello"},     // exact length — not truncated
		{"hello world", 5, "hello…"}, // over limit — truncated with ellipsis
		{"", 5, ""},               // empty string
		{"héllo world", 5, "héllo…"}, // multi-byte runes — "héllo" is 5 runes
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	t.Run("zero time returns dash", func(t *testing.T) {
		if got := formatTime(time.Time{}); got != "-" {
			t.Errorf("formatTime(zero) = %q, want \"-\"", got)
		}
	})

	t.Run("non-zero time returns non-empty non-dash string", func(t *testing.T) {
		ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
		got := formatTime(ts)
		if got == "" || got == "-" {
			t.Errorf("formatTime(%v) = %q, expected formatted timestamp", ts, got)
		}
	})
}

func TestPrintEligibleTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	PrintEligibleTable(&buf, nil)
	if !strings.Contains(buf.String(), "No eligible assignments found.") {
		t.Errorf("expected empty message, got: %q", buf.String())
	}
}

func TestPrintEligibleTableRow(t *testing.T) {
	var buf bytes.Buffer
	assignments := []pim.EligibleAssignment{
		{
			RoleName:       "Contributor",
			ScopeDisplay:   "my-subscription",
			ResourceType:   "Subscription",
			MembershipType: "Direct",
		},
	}
	PrintEligibleTable(&buf, assignments)
	if !strings.Contains(buf.String(), "Contributor") {
		t.Errorf("expected role name in output, got: %q", buf.String())
	}
}

func TestPrintActiveTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	PrintActiveTable(&buf, nil, false)
	if !strings.Contains(buf.String(), "No active assignments found.") {
		t.Errorf("expected empty message, got: %q", buf.String())
	}
}

func TestPrintActiveTableRow(t *testing.T) {
	var buf bytes.Buffer
	assignments := []pim.ActiveAssignment{
		{
			RoleName:       "Owner",
			Resource:       "my-subscription",
			ResourceType:   "Subscription",
			MembershipType: "Direct",
			State:          "Active",
			HasExpiry:      true,
			EndTime:        time.Now().Add(4 * time.Hour),
		},
	}
	PrintActiveTable(&buf, assignments, false)
	if !strings.Contains(buf.String(), "Owner") {
		t.Errorf("expected role name in output, got: %q", buf.String())
	}
}
```

**Step 2: Run tests to verify they pass**

```bash
cd azpim && go test ./internal/output/ -run "TestTruncate|TestFormatTime|TestPrintEligibleTable|TestPrintActiveTable" -v
```

Expected: all PASS

**Step 3: Commit**

```bash
git add internal/output/table_test.go
git commit -m "test: add output/table tests (truncate, formatTime, PrintEligibleTable, PrintActiveTable)"
```

---

## Task 7: `internal/output/json_test.go` — PrintEligibleJSON, PrintActiveJSON

**Files:**
- Create: `azpim/internal/output/json_test.go`

**Step 1: Create the test file**

```go
package output

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"azpim/internal/pim"
)

func TestPrintEligibleJSON(t *testing.T) {
	assignments := []pim.EligibleAssignment{
		{
			RoleName:       "Contributor",
			Scope:          "/subscriptions/sub1",
			ScopeDisplay:   "My Sub",
			ResourceType:   "Subscription",
			MembershipType: "Direct",
			HasExpiry:      false,
			RoleDefID:      "/providers/Microsoft.Authorization/roleDefinitions/abc",
		},
	}
	var buf bytes.Buffer
	if err := PrintEligibleJSON(&buf, assignments); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d entries, want 1", len(out))
	}
	if out[0]["role_name"] != "Contributor" {
		t.Errorf("role_name = %v, want Contributor", out[0]["role_name"])
	}
	if out[0]["scope"] != "/subscriptions/sub1" {
		t.Errorf("scope = %v, want /subscriptions/sub1", out[0]["scope"])
	}
	if out[0]["has_expiry"] != false {
		t.Errorf("has_expiry = %v, want false", out[0]["has_expiry"])
	}
}

func TestPrintEligibleJSONWithExpiry(t *testing.T) {
	end := time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC)
	assignments := []pim.EligibleAssignment{
		{RoleName: "Owner", HasExpiry: true, EndTime: end},
	}
	var buf bytes.Buffer
	if err := PrintEligibleJSON(&buf, assignments); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out[0]["has_expiry"] != true {
		t.Errorf("has_expiry = %v, want true", out[0]["has_expiry"])
	}
	if _, ok := out[0]["end_time"]; !ok {
		t.Error("expected end_time field to be present")
	}
}

func TestPrintActiveJSON(t *testing.T) {
	end := time.Now().Add(4 * time.Hour)
	assignments := []pim.ActiveAssignment{
		{
			RoleName:       "Contributor",
			Resource:       "My Sub",
			ResourceType:   "Subscription",
			MembershipType: "Direct",
			State:          "Active",
			HasExpiry:      true,
			EndTime:        end,
			Scope:          "/subscriptions/sub1",
			RoleDefID:      "/providers/Microsoft.Authorization/roleDefinitions/abc",
		},
	}
	var buf bytes.Buffer
	if err := PrintActiveJSON(&buf, assignments, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d entries, want 1", len(out))
	}
	if out[0]["role_name"] != "Contributor" {
		t.Errorf("role_name = %v, want Contributor", out[0]["role_name"])
	}
	if out[0]["state"] != "Active" {
		t.Errorf("state = %v, want Active", out[0]["state"])
	}
	remaining, ok := out[0]["time_remaining_seconds"].(float64)
	if !ok || remaining <= 0 {
		t.Errorf("expected positive time_remaining_seconds, got %v", out[0]["time_remaining_seconds"])
	}
}
```

**Step 2: Run tests to verify they pass**

```bash
cd azpim && go test ./internal/output/ -run "TestPrintEligibleJSON|TestPrintActiveJSON" -v
```

Expected: all PASS

**Step 3: Commit**

```bash
git add internal/output/json_test.go
git commit -m "test: add output/json tests (PrintEligibleJSON, PrintActiveJSON)"
```

---

## Task 8: Extend `cmd/activate_test.go` — resolveDuration, selectEligible, printActivateJSON

**Files:**
- Modify: `azpim/cmd/activate_test.go`

**Step 1: Add imports and append test functions**

Update the import block at the top of `activate_test.go` to add `"bytes"` and `"encoding/json"`:

```go
import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"azpim/internal/pim"
)
```

Then append these test functions at the end of the file:

```go
func TestResolveDuration(t *testing.T) {
	tests := []struct {
		flag    string
		maxDur  time.Duration
		want    time.Duration
		wantErr bool
	}{
		{"4h", 8 * time.Hour, 4 * time.Hour, false},
		{"4h", 2 * time.Hour, 0, true},  // exceeds max
		{"0", 8 * time.Hour, 0, true},   // zero hours → not positive
		{"4h30m", 0, 4*time.Hour + 30*time.Minute, false}, // no max restriction
		{"invalid", 8 * time.Hour, 0, true},
	}
	for _, tt := range tests {
		got, err := resolveDuration(tt.flag, tt.maxDur)
		if (err != nil) != tt.wantErr {
			t.Errorf("resolveDuration(%q, %v): err=%v, wantErr=%v", tt.flag, tt.maxDur, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("resolveDuration(%q, %v) = %v, want %v", tt.flag, tt.maxDur, got, tt.want)
		}
	}
}

func TestSelectEligible(t *testing.T) {
	newEntry := func(name, guid, scope string) pim.EligibleAssignment {
		return pim.EligibleAssignment{
			RoleName:  name,
			RoleDefID: "/providers/Microsoft.Authorization/roleDefinitions/" + guid,
			Scope:     scope,
		}
	}
	eligible := []pim.EligibleAssignment{
		newEntry("Owner", "guid-owner", "/subscriptions/sub1"),
		newEntry("Contributor", "guid-contrib", "/subscriptions/sub1"),
	}

	t.Run("single match returns it", func(t *testing.T) {
		got, err := selectEligible(eligible, "Owner")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.RoleName != "Owner" {
			t.Errorf("got role %q, want Owner", got.RoleName)
		}
	})

	t.Run("no match returns error", func(t *testing.T) {
		_, err := selectEligible(eligible, "NonExistent")
		if err == nil {
			t.Error("expected error for no match, got nil")
		}
	})
}

func TestPrintActivateJSON(t *testing.T) {
	var buf bytes.Buffer
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	dur := 4 * time.Hour
	err := printActivateJSON(&buf,
		"Owner",
		"/providers/Microsoft.Authorization/roleDefinitions/abc",
		"/subscriptions/sub1",
		"My Sub",
		dur,
		now,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["role_name"] != "Owner" {
		t.Errorf("role_name = %v, want Owner", out["role_name"])
	}
	if out["scope_display"] != "My Sub" {
		t.Errorf("scope_display = %v, want My Sub", out["scope_display"])
	}
	if out["duration_seconds"] != float64(dur.Seconds()) {
		t.Errorf("duration_seconds = %v, want %v", out["duration_seconds"], dur.Seconds())
	}
	activatedAt, _ := out["activated_at"].(string)
	expiresAt, _ := out["expires_at"].(string)
	activated, _ := time.Parse(time.RFC3339, activatedAt)
	expires, _ := time.Parse(time.RFC3339, expiresAt)
	if diff := expires.Sub(activated); diff != dur {
		t.Errorf("expires_at - activated_at = %v, want %v", diff, dur)
	}
}
```

**Step 2: Run tests to verify they pass**

```bash
cd azpim && go test ./cmd/ -run "TestResolveDuration|TestSelectEligible|TestPrintActivateJSON" -v
```

Expected: all PASS

**Step 3: Run all tests to ensure nothing is broken**

```bash
cd azpim && go test ./...
```

Expected: all packages PASS

**Step 4: Commit**

```bash
git add cmd/activate_test.go
git commit -m "test: add resolveDuration, selectEligible, printActivateJSON tests"
```

---

## Final Step: Verify coverage

```bash
cd azpim && go test ./... -cover
```

Review the coverage output across packages. All pure functions should now be covered.
