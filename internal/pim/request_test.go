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
		// Seconds are rounded to the nearest minute (Azure PIM has no sub-minute granularity).
		{1*time.Hour + 30*time.Minute + 30*time.Second, "PT1H31M"}, // rounds up
		{1*time.Hour + 30*time.Minute + 29*time.Second, "PT1H30M"}, // rounds down
		{4*time.Hour + 29*time.Second, "PT4H"},                     // rounds down to whole hour
	}
	for _, tt := range tests {
		got := durationToISO8601(tt.d)
		if got != tt.want {
			t.Errorf("durationToISO8601(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
