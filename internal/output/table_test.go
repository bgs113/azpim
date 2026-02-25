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
