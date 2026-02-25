package pim

import (
	"testing"
	"time"
)

func TestParseISO8601Duration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"PT8H", 8 * time.Hour, false},
		{"PT1H", time.Hour, false},
		{"PT30M", 30 * time.Minute, false},
		{"PT1M", time.Minute, false},
		{"PT8H30M", 8*time.Hour + 30*time.Minute, false},
		{"PT1H30M", time.Hour + 30*time.Minute, false},
		{"P1D", 24 * time.Hour, false},
		{"invalid", 0, true},  // no P prefix
		{"8H", 0, true},       // no P prefix
		{"PTXH", 0, true},     // non-numeric
	}
	for _, tt := range tests {
		got, err := parseISO8601Duration(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseISO8601Duration(%q): err = %v, wantErr = %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("parseISO8601Duration(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// Round-trip: durationToISO8601 output should parse back to the original value.
func TestDurationISO8601RoundTrip(t *testing.T) {
	durations := []time.Duration{
		time.Hour,
		4 * time.Hour,
		30 * time.Minute,
		4*time.Hour + 30*time.Minute,
		8 * time.Hour,
	}
	for _, d := range durations {
		iso := durationToISO8601(d)
		got, err := parseISO8601Duration(iso)
		if err != nil {
			t.Errorf("parseISO8601Duration(%q) unexpected error: %v", iso, err)
			continue
		}
		if got != d {
			t.Errorf("round-trip %v → %q → %v", d, iso, got)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{8 * time.Hour, "8h"},
		{1 * time.Hour, "1h"},
		{time.Hour + 30*time.Minute, "1h30m"},
		{4*time.Hour + 30*time.Minute, "4h30m"},
		{30 * time.Minute, "0h30m"}, // sub-hour: h=0 so produces "0h30m"
	}
	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
