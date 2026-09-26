package pim

import (
	"strings"
	"testing"
)

func TestMatchManagementGroup(t *testing.T) {
	mg := func(id, name string) scopeEntry {
		return scopeEntry{Scope: "/providers/Microsoft.Management/managementGroups/" + id, DisplayName: name}
	}
	entries := []scopeEntry{
		mg("mg-prod", "Production"),
		mg("mg-prod", "Production"), // eligible and active at the same group
		mg("mg-dev-1", "Dev"),
		mg("mg-dev-2", "Dev"),
		{Scope: "/subscriptions/s1", DisplayName: "Sandbox"}, // not a management group
	}
	tests := []struct {
		name, want, wantErr string
	}{
		{"Production", "/providers/Microsoft.Management/managementGroups/mg-prod", ""},
		{"production", "/providers/Microsoft.Management/managementGroups/mg-prod", ""},
		{"Dev", "", "2 management groups are named \"Dev\""},
		{"Staging", "", "Production (ID mg-prod)"},
		{"Sandbox", "", "no management group with ID or name \"Sandbox\""},
	}
	for _, tt := range tests {
		got, err := matchManagementGroup(entries, tt.name)
		if tt.wantErr == "" {
			if err != nil || got != tt.want {
				t.Errorf("%q: got %q, %v; want %q", tt.name, got, err, tt.want)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("%q: err = %v; want it to contain %q", tt.name, err, tt.wantErr)
		}
	}
	// The ambiguous error lists each candidate's ID.
	_, err := matchManagementGroup(entries, "Dev")
	if !strings.Contains(err.Error(), "mg-dev-1") || !strings.Contains(err.Error(), "mg-dev-2") {
		t.Errorf("ambiguous error should list both IDs: %v", err)
	}
}
