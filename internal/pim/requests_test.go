package pim

import (
	"testing"

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

func TestDeduplicateRequests(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		if got := deduplicateRequests(nil); len(got) != 0 {
			t.Errorf("got %d, want 0", len(got))
		}
	})

	t.Run("no duplicates unchanged", func(t *testing.T) {
		in := []ScheduleRequestEntry{
			{RequestName: "uuid-1", RoleDefID: "role1", Status: "Active"},
			{RequestName: "uuid-2", RoleDefID: "role2", Status: "Pending"},
		}
		if got := deduplicateRequests(in); len(got) != 2 {
			t.Errorf("got %d, want 2", len(got))
		}
	})

	t.Run("duplicate request names collapsed", func(t *testing.T) {
		in := []ScheduleRequestEntry{
			{RequestName: "uuid-1", RoleDefID: "role1"},
			{RequestName: "uuid-1", RoleDefID: "role1"}, // same request from second scope query
		}
		if got := deduplicateRequests(in); len(got) != 1 {
			t.Errorf("got %d, want 1", len(got))
		}
	})

	t.Run("empty request name falls back to composite key", func(t *testing.T) {
		in := []ScheduleRequestEntry{
			{RequestName: "", RoleDefID: "role1", Scope: "/subscriptions/sub1", RequestType: "Activate"},
			{RequestName: "", RoleDefID: "role1", Scope: "/subscriptions/sub1", RequestType: "Activate"},
		}
		if got := deduplicateRequests(in); len(got) != 1 {
			t.Errorf("got %d, want 1", len(got))
		}
	})
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
