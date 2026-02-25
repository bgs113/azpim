package pim

import (
	"testing"
	"time"
)

func TestDurationToISO8601(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{4 * time.Hour, "PT4H"},
		{8 * time.Hour, "PT8H"},
		{30 * time.Minute, "PT30M"},
		{1 * time.Minute, "PT1M"},
		{4*time.Hour + 30*time.Minute, "PT4H30M"},
		{1*time.Hour + 30*time.Minute, "PT1H30M"},
	}
	for _, tt := range tests {
		got := durationToISO8601(tt.d)
		if got != tt.want {
			t.Errorf("durationToISO8601(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
