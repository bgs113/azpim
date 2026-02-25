package cmd

import (
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
