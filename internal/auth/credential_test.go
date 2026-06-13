package auth

import "testing"

func TestExtractJSONStringValue(t *testing.T) {
	tests := []struct {
		json string
		key  string
		want string
	}{
		{`{"oid":"abc123","tid":"tenant1"}`, "oid", "abc123"},
		{`{"oid":"abc123","tid":"tenant1"}`, "tid", "tenant1"},
		{`{"oid":"abc123"}`, "tid", ""}, // missing key
		{`{}`, "oid", ""},
		{`{"oid":""}`, "oid", ""},        // empty value
		{`{"oid":"a\"b"}`, "oid", "a\\"}, // stops at first unescaped quote
	}
	for _, tt := range tests {
		got := extractJSONStringValue(tt.json, tt.key)
		if got != tt.want {
			t.Errorf("extractJSONStringValue(%q, %q) = %q, want %q", tt.json, tt.key, got, tt.want)
		}
	}
}
