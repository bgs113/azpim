package cmd

import (
	"errors"
	"fmt"
	"testing"
)

func TestArmCodeDesc(t *testing.T) {
	tests := []struct {
		code   string
		status int
		want   string
	}{
		{"RoleAssignmentExists", 409, "role is already active"},
		{"RoleAssignmentAlreadyExists", 409, "role is already active"},
		{"RoleAssignmentScheduleRequestExists", 409, "an activation request for this role is already pending"},
		{"RoleEligibilityDoesNotExist", 404, "no eligible assignment found at this scope"},
		{"PendingApproval", 400, "role requires admin approval before it can be activated"},
		{"PendingAdminDecision", 400, "role requires admin approval before it can be activated"},
		{"AuthorizationFailed", 403, "permission denied"},
		{"InvalidScope", 400, "invalid scope"},
		{"UnknownCode", 400, "UnknownCode (HTTP 400)"},
		{"", 404, "HTTP 404"},
	}
	for _, tt := range tests {
		got := armCodeDesc(tt.code, tt.status)
		if got != tt.want {
			t.Errorf("armCodeDesc(%q, %d) = %q, want %q", tt.code, tt.status, got, tt.want)
		}
	}
}

func TestFriendlyError(t *testing.T) {
	t.Run("expired credentials", func(t *testing.T) {
		err := errors.New("DefaultAzureCredential: token has expired AADSTS50173")
		got := friendlyError(err)
		want := "Azure credentials have expired — run 'az login' to re-authenticate"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("auth failure", func(t *testing.T) {
		err := errors.New("DefaultAzureCredential: failed to acquire token")
		got := friendlyError(err)
		want := "Azure authentication failed — run 'az login' to sign in"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("plain error passthrough", func(t *testing.T) {
		msg := "something went wrong"
		err := errors.New(msg)
		got := friendlyError(err)
		if got != msg {
			t.Errorf("got %q, want %q", got, msg)
		}
	})

	t.Run("wrapped plain error passthrough", func(t *testing.T) {
		inner := errors.New("inner error")
		err := fmt.Errorf("outer: %w", inner)
		got := friendlyError(err)
		if got != err.Error() {
			t.Errorf("got %q, want %q", got, err.Error())
		}
	})
}
