package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
	_ "time/tzdata" // parseStart tests use a named zone, independent of the machine's

	"github.com/bgs113/azpim/internal/pim"
)

func TestParseDurationInput(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"4", 4 * time.Hour, false},
		{"8", 8 * time.Hour, false},
		{"1", 1 * time.Hour, false},
		{"4h", 4 * time.Hour, false},
		{"4h30m", 4*time.Hour + 30*time.Minute, false},
		{"90m", 90 * time.Minute, false},
		{"30m", 30 * time.Minute, false},
		{"  4h  ", 4 * time.Hour, false}, // whitespace trimmed
		{"", 0, true},
		{"invalid", 0, true},
		{"4x", 0, true},
	}
	for _, tt := range tests {
		d, err := parseDurationInput(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseDurationInput(%q): err = %v, wantErr = %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && d != tt.want {
			t.Errorf("parseDurationInput(%q) = %v, want %v", tt.input, d, tt.want)
		}
	}
}

func TestMatchEligible(t *testing.T) {
	newEntry := func(name, guid, scope, membership string) pim.EligibleAssignment {
		return pim.EligibleAssignment{
			RoleName:       name,
			RoleDefID:      "/providers/Microsoft.Authorization/roleDefinitions/" + guid,
			Scope:          scope,
			MembershipType: membership,
		}
	}

	contrib1 := newEntry("Contributor", "guid-contrib", "/subscriptions/sub1", "Direct")
	contrib2 := newEntry("Contributor", "guid-contrib", "/subscriptions/sub2", "Direct")
	owner := newEntry("Owner", "guid-owner", "/subscriptions/sub1", "Direct")
	reader := newEntry("Storage Blob Data Reader", "guid-reader", "/subscriptions/sub1", "Direct")

	eligible := []pim.EligibleAssignment{contrib1, contrib2, owner, reader}

	t.Run("exact match single result", func(t *testing.T) {
		got := matchByRoleName(eligible, "Owner", eligibleName)
		if len(got) != 1 || got[0].RoleName != "Owner" {
			t.Errorf("got %v, want [Owner]", roleNames(got))
		}
	})

	t.Run("exact match case insensitive", func(t *testing.T) {
		got := matchByRoleName(eligible, "owner", eligibleName)
		if len(got) != 1 || got[0].RoleName != "Owner" {
			t.Errorf("got %v, want [Owner]", roleNames(got))
		}
	})

	t.Run("exact match multiple scopes", func(t *testing.T) {
		got := matchByRoleName(eligible, "Contributor", eligibleName)
		if len(got) != 2 {
			t.Errorf("got %d matches, want 2", len(got))
		}
	})

	t.Run("prefix match fallback", func(t *testing.T) {
		got := matchByRoleName(eligible, "Storage", eligibleName)
		if len(got) != 1 || got[0].RoleName != "Storage Blob Data Reader" {
			t.Errorf("got %v, want [Storage Blob Data Reader]", roleNames(got))
		}
	})

	t.Run("exact match beats prefix", func(t *testing.T) {
		// "Owner" exact-matches "Owner" and is also a prefix of nothing else here,
		// but adding "Owner Extended" checks that exact takes priority.
		ownerExt := newEntry("Owner Extended", "guid-ownerext", "/subscriptions/sub1", "Direct")
		got := matchByRoleName(append(eligible, ownerExt), "Owner", eligibleName)
		if len(got) != 1 || got[0].RoleName != "Owner" {
			t.Errorf("got %v, want exact [Owner] only", roleNames(got))
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got := matchByRoleName(eligible, "NonExistent", eligibleName)
		if len(got) != 0 {
			t.Errorf("got %v, want empty", roleNames(got))
		}
	})
}

func eligibleName(a pim.EligibleAssignment) string { return a.RoleName }

func roleNames(assignments []pim.EligibleAssignment) []string {
	names := make([]string, len(assignments))
	for i, a := range assignments {
		names[i] = a.RoleName
	}
	return names
}

func TestResolveDuration(t *testing.T) {
	tests := []struct {
		flag    string
		maxDur  time.Duration
		want    time.Duration
		wantErr bool
	}{
		{"4h", 8 * time.Hour, 4 * time.Hour, false},
		{"4h", 2 * time.Hour, 0, true},                    // exceeds max
		{"0", 8 * time.Hour, 0, true},                     // zero hours → not positive
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
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	dur := 4 * time.Hour

	t.Run("active status", func(t *testing.T) {
		var buf bytes.Buffer
		err := printActivateJSON(&buf,
			"Owner",
			"/providers/Microsoft.Authorization/roleDefinitions/abc",
			"/subscriptions/sub1",
			"My Sub",
			dur,
			now,
			time.Time{},
			pim.RequestOutcome{Status: "Provisioned"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var out map[string]any
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
		if out["status"] != "Active" {
			t.Errorf("status = %v, want Active", out["status"])
		}
		activatedAt, _ := out["activated_at"].(string)
		expiresAt, _ := out["expires_at"].(string)
		activated, _ := time.Parse(time.RFC3339, activatedAt)
		expires, _ := time.Parse(time.RFC3339, expiresAt)
		if diff := expires.Sub(activated); diff != dur {
			t.Errorf("expires_at - activated_at = %v, want %v", diff, dur)
		}
	})

	t.Run("pending approval status", func(t *testing.T) {
		var buf bytes.Buffer
		err := printActivateJSON(&buf,
			"Contributor",
			"/providers/Microsoft.Authorization/roleDefinitions/def",
			"/subscriptions/sub1",
			"My Sub",
			dur,
			now,
			time.Time{},
			pim.RequestOutcome{Status: "PendingAdminDecision"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var out map[string]any
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out["status"] != "Pending" || out["azure_status"] != "PendingAdminDecision" {
			t.Errorf("status = %v, azure_status = %v; want Pending, PendingAdminDecision", out["status"], out["azure_status"])
		}
		if _, ok := out["expires_at"]; ok {
			t.Errorf("expires_at = %v; want it omitted while the request is pending", out["expires_at"])
		}
	})
}

func TestOutcomeLine(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"Provisioned", "✓ done"},
		{"PendingAdminDecision", `Extension request for "Reader" submitted and pending (Azure status: PendingAdminDecision). It is not in effect yet — see 'azpim requests --pending'.`},
		{"Accepted", `Extension request for "Reader" submitted, but Azure did not confirm it took effect (status: Accepted). Check 'azpim requests' before relying on it.`},
		{"", `Extension request for "Reader" submitted, but Azure did not confirm it took effect (status: none). Check 'azpim requests' before relying on it.`},
	}
	for _, tt := range tests {
		got := outcomeLine(pim.RequestOutcome{Status: tt.status}, "Extension", "Reader", "✓ done")
		if got != tt.want {
			t.Errorf("outcomeLine(%q) =\n  %s\nwant\n  %s", tt.status, got, tt.want)
		}
	}
}

func TestOutcomeLineChecks(t *testing.T) {
	if got, want := outcomeLine(pim.RequestOutcome{Check: "DryRun"}, "Extension", "Reader", "✓ done"),
		`Dry run: extension request for "Reader" not sent.`; got != want {
		t.Errorf("dry run: got %q, want %q", got, want)
	}
	// A validated request must not claim success even though Azure's status is Provisioned.
	if got, want := outcomeLine(pim.RequestOutcome{Check: "Validated", Status: "Provisioned"}, "Extension", "Reader", "✓ done"),
		`✓ Extension request for "Reader" passed Azure's validation. Nothing was submitted.`; got != want {
		t.Errorf("validated: got %q, want %q", got, want)
	}
}

func TestPrintActivateJSONChecks(t *testing.T) {
	for _, check := range []string{"DryRun", "Validated"} {
		var buf bytes.Buffer
		if err := printActivateJSON(&buf, "Owner", "rd", "/subscriptions/s", "S", time.Hour, time.Now(), time.Time{},
			pim.RequestOutcome{Check: check, Status: "Provisioned"}); err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out["status"] != check {
			t.Errorf("status = %v, want %s", out["status"], check)
		}
		if _, ok := out["activated_at"]; ok {
			t.Errorf("%s: activated_at set; nothing was activated", check)
		}
	}
}

func TestParseStart(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 20:00 EST on the evening before DST starts (2026-03-08 02:00).
	now := time.Date(2026, 3, 7, 20, 0, 0, 0, ny)
	tests := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"22:00", time.Date(2026, 3, 7, 22, 0, 0, 0, ny), false},
		// Already past today, so tomorrow; after DST starts, 09:00 EDT is 13:00 UTC (Add(24h) would give 10:00).
		{"09:00", time.Date(2026, 3, 8, 13, 0, 0, 0, time.UTC), false},
		{"2026-10-01T22:00", time.Date(2026, 10, 2, 2, 0, 0, 0, time.UTC), false},
		{"2026-10-01 22:00", time.Date(2026, 10, 2, 2, 0, 0, 0, time.UTC), false},
		{"2026-10-01T22:00:00Z", time.Date(2026, 10, 1, 22, 0, 0, 0, time.UTC), false},
		{"2026-10-01T22:00:00+02:00", time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC), false},
		{"2026-03-07T19:57", time.Date(2026, 3, 8, 0, 57, 0, 0, time.UTC), false}, // 3 minutes ago: within the clock-skew grace
		{"2026-03-07T19:00", time.Time{}, true},                                   // an hour ago
		{"tomorrow", time.Time{}, true},
		{"25:00", time.Time{}, true},
	}
	for _, tt := range tests {
		got, err := parseStart(tt.in, now)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseStart(%q): err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && !got.Equal(tt.want) {
			t.Errorf("parseStart(%q) = %v, want %v", tt.in, got.UTC(), tt.want.UTC())
		}
	}
}

func TestPrintActivateJSONScheduled(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	start := time.Date(2026, 10, 1, 22, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	if err := printActivateJSON(&buf, "Owner", "rd", "/subscriptions/s", "S", 4*time.Hour, now, start,
		pim.RequestOutcome{Status: "PendingScheduleCreation"}); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["requested_at"] != "2026-09-25T12:00:00Z" || out["activated_at"] != "2026-10-01T22:00:00Z" || out["expires_at"] != "2026-10-02T02:00:00Z" {
		t.Errorf("requested_at/activated_at/expires_at = %v/%v/%v", out["requested_at"], out["activated_at"], out["expires_at"])
	}

	// Waiting for an approver is not "scheduled": no times until it is approved.
	buf.Reset()
	if err := printActivateJSON(&buf, "Owner", "rd", "/subscriptions/s", "S", 4*time.Hour, now, start,
		pim.RequestOutcome{Status: "PendingAdminDecision"}); err != nil {
		t.Fatal(err)
	}
	out = nil
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if _, ok := out["activated_at"]; ok {
		t.Errorf("activated_at set while awaiting approval")
	}
}

// --start with --dry-run or --validate-only creates nothing, so it must not
// report a schedule, even if the validate response says PendingScheduleCreation.
func TestScheduledChecks(t *testing.T) {
	start := time.Date(2026, 10, 1, 22, 0, 0, 0, time.UTC)
	for _, o := range []pim.RequestOutcome{
		{Check: "DryRun"},
		{Check: "Validated", Status: "PendingScheduleCreation"},
		{Check: "Validated", Status: "Provisioned"},
	} {
		if o.Scheduled() {
			t.Errorf("%+v: Scheduled() = true; nothing was created", o)
		}
		var buf bytes.Buffer
		if err := printActivateJSON(&buf, "Owner", "rd", "/subscriptions/s", "S", time.Hour, time.Now(), start, o); err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if _, ok := out["activated_at"]; ok || out["status"] != o.Check {
			t.Errorf("%+v: activated_at present = %v, status = %v; want absent, %s", o, ok, out["status"], o.Check)
		}
	}
}
