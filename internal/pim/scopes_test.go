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
