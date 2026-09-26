package pim

import (
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
)

func ptrRequestType(rt armauthorization.RequestType) *armauthorization.RequestType { return &rt }
func ptrStatus(s armauthorization.Status) *armauthorization.Status                 { return &s }

func TestHumanizeRequestType(t *testing.T) {
	tests := []struct {
		rt   *armauthorization.RequestType
		want string
	}{
		{ptrRequestType(armauthorization.RequestTypeSelfActivate), "Activate"},
		{ptrRequestType(armauthorization.RequestTypeSelfExtend), "Extend"},
		{ptrRequestType(armauthorization.RequestTypeSelfDeactivate), "Deactivate"},
		{ptrRequestType(armauthorization.RequestTypeAdminAssign), "AdminAssign"},
		{nil, ""},
	}
	for _, tt := range tests {
		got := humanizeRequestType(tt.rt)
		if got != tt.want {
			t.Errorf("humanizeRequestType(%v) = %q, want %q", tt.rt, got, tt.want)
		}
	}
}

func TestHumanizeRequestStatus(t *testing.T) {
	tests := []struct {
		s    *armauthorization.Status
		want string
	}{
		{ptrStatus(armauthorization.StatusProvisioned), "Active"},
		{ptrStatus(armauthorization.StatusAdminApproved), "Active"},
		{ptrStatus(armauthorization.StatusScheduleCreated), "Active"},
		{ptrStatus(armauthorization.StatusPendingApproval), "Pending"},
		{ptrStatus(armauthorization.StatusPendingAdminDecision), "Pending"},
		{ptrStatus(armauthorization.StatusPendingEvaluation), "Pending"},
		{ptrStatus(armauthorization.StatusPendingProvisioning), "Pending"},
		{ptrStatus(armauthorization.StatusPendingScheduleCreation), "Pending"},
		{ptrStatus(armauthorization.StatusAdminDenied), "Denied"},
		{ptrStatus(armauthorization.StatusDenied), "Denied"},
		{ptrStatus(armauthorization.StatusFailed), "Failed"},
		{ptrStatus(armauthorization.StatusCanceled), "Canceled"},
		{ptrStatus(armauthorization.StatusRevoked), "Revoked"},
		{nil, ""},
	}
	for _, tt := range tests {
		got := humanizeRequestStatus(tt.s)
		if got != tt.want {
			t.Errorf("humanizeRequestStatus(%v) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestScheduleRequestEntryIsPending(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"Pending", true},
		{"Active", false},
		{"Denied", false},
		{"Failed", false},
		{"Canceled", false},
		{"", false},
	}
	for _, tt := range tests {
		r := ScheduleRequestEntry{Status: tt.status}
		if got := r.Status == "Pending"; got != tt.want {
			t.Errorf("Status==%q: got %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestRequestState(t *testing.T) {
	st := ptrStatus
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	tests := []struct {
		status *armauthorization.Status
		start  time.Time
		want   string
	}{
		{st(armauthorization.StatusPendingScheduleCreation), future, "Scheduled"},
		{st(armauthorization.StatusProvisioned), future, "Scheduled"},
		{st(armauthorization.StatusProvisioned), past, "Active"},
		{st(armauthorization.StatusPendingAdminDecision), future, "Pending"}, // still needs an approver
		{st(armauthorization.StatusDenied), future, "Denied"},
		{nil, future, ""},
	}
	for _, tt := range tests {
		if got := requestState(tt.status, tt.start, now); got != tt.want {
			t.Errorf("requestState(%v, start in %v) = %q, want %q", tt.status, tt.start.Sub(now), got, tt.want)
		}
	}
}
