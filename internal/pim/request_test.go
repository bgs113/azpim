package pim

import (
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
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

func TestOutcomeFromStatus(t *testing.T) {
	st := func(s armauthorization.Status) *armauthorization.Status { return &s }
	tests := []struct {
		in         *armauthorization.Status
		done, pend bool
		wantErr    bool
		wantStatus string
	}{
		{st(armauthorization.StatusProvisioned), true, false, false, "Provisioned"},
		{st(armauthorization.StatusGranted), true, false, false, "Granted"},
		{st(armauthorization.StatusRevoked), true, false, false, "Revoked"},
		{st(armauthorization.StatusPendingAdminDecision), false, true, false, "PendingAdminDecision"},
		{st(armauthorization.StatusPendingApproval), false, true, false, "PendingApproval"},
		{st(armauthorization.StatusPendingRevocation), false, true, false, "PendingRevocation"},
		{st(armauthorization.StatusProvisioningStarted), false, true, false, "ProvisioningStarted"},
		{st(armauthorization.StatusAccepted), false, false, false, "Accepted"},
		{nil, false, false, false, ""},
		{st(armauthorization.StatusDenied), false, false, true, "Denied"},
		{st(armauthorization.StatusAdminDenied), false, false, true, "AdminDenied"},
		{st(armauthorization.StatusFailed), false, false, true, "Failed"},
		{st(armauthorization.StatusTimedOut), false, false, true, "TimedOut"},
		{st(armauthorization.StatusCanceled), false, false, true, "Canceled"},
	}
	for _, tt := range tests {
		o, err := outcomeFromStatus(tt.in)
		if o.Status != tt.wantStatus || o.Done() != tt.done || o.Pending() != tt.pend || (err != nil) != tt.wantErr {
			t.Errorf("outcomeFromStatus(%q) = {Status:%q Done:%v Pending:%v} err=%v; want Status %q Done %v Pending %v err %v",
				tt.wantStatus, o.Status, o.Done(), o.Pending(), err, tt.wantStatus, tt.done, tt.pend, tt.wantErr)
		}
	}
}
