package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"azpim/internal/pim"
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
		got := matchEligible(eligible, "Owner")
		if len(got) != 1 || got[0].RoleName != "Owner" {
			t.Errorf("got %v, want [Owner]", roleNames(got))
		}
	})

	t.Run("exact match case insensitive", func(t *testing.T) {
		got := matchEligible(eligible, "owner")
		if len(got) != 1 || got[0].RoleName != "Owner" {
			t.Errorf("got %v, want [Owner]", roleNames(got))
		}
	})

	t.Run("exact match multiple scopes", func(t *testing.T) {
		got := matchEligible(eligible, "Contributor")
		if len(got) != 2 {
			t.Errorf("got %d matches, want 2", len(got))
		}
	})

	t.Run("prefix match fallback", func(t *testing.T) {
		got := matchEligible(eligible, "Storage")
		if len(got) != 1 || got[0].RoleName != "Storage Blob Data Reader" {
			t.Errorf("got %v, want [Storage Blob Data Reader]", roleNames(got))
		}
	})

	t.Run("exact match beats prefix", func(t *testing.T) {
		// "Owner" exact-matches "Owner" and is also a prefix of nothing else here,
		// but adding "Owner Extended" checks that exact takes priority.
		ownerExt := newEntry("Owner Extended", "guid-ownerext", "/subscriptions/sub1", "Direct")
		got := matchEligible(append(eligible, ownerExt), "Owner")
		if len(got) != 1 || got[0].RoleName != "Owner" {
			t.Errorf("got %v, want exact [Owner] only", roleNames(got))
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got := matchEligible(eligible, "NonExistent")
		if len(got) != 0 {
			t.Errorf("got %v, want empty", roleNames(got))
		}
	})
}

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
	activatedAt, _ := out["activated_at"].(string)
	expiresAt, _ := out["expires_at"].(string)
	activated, _ := time.Parse(time.RFC3339, activatedAt)
	expires, _ := time.Parse(time.RFC3339, expiresAt)
	if diff := expires.Sub(activated); diff != dur {
		t.Errorf("expires_at - activated_at = %v, want %v", diff, dur)
	}
}
