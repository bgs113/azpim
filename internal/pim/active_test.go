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
			t.Errorf("TimeRemaining(false) = %q, want Expired", got)
		}
		if got := a.TimeRemaining(true); got != "Expired" {
			t.Errorf("TimeRemaining(true) = %q, want Expired", got)
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
