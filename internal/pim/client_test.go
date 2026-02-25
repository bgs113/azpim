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
