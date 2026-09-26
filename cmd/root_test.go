package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
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
		{"RoleAssignmentDoesNotExist", 400, "no active assignment for this role at this scope — Azure rejects changes for about 5 minutes after activation, so if you just activated it, wait and retry"},
		{"PendingRoleAssignmentRequest", 400, "a request for this role is already pending — see 'azpim requests --pending'"},
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

func TestMgIDRe(t *testing.T) {
	for in, want := range map[string]bool{
		"mg-prod":                              true,
		"Production":                           true,
		"Contoso_(EU).1":                       true,
		"72f988bf-86f1-41af-91ab-2d7cd011db47": true,
		"Production Workloads":                 false,
		"Prod/Sub":                             false,
		"":                                     false,
	} {
		if got := mgIDRe.MatchString(in); got != want {
			t.Errorf("mgIDRe(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{&azcore.ResponseError{StatusCode: 404}, true},
		{fmt.Errorf("list: %w", &azcore.ResponseError{StatusCode: 400, ErrorCode: "InvalidScope"}), true},
		{&azcore.ResponseError{StatusCode: 403, ErrorCode: "AuthorizationFailed"}, false},
		{errors.New("network down"), false},
	}
	for _, tt := range tests {
		if got := isNotFound(tt.err); got != tt.want {
			t.Errorf("isNotFound(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

// listAt must not look anything up unless -m could be a name and ARM said not found.
// clients is nil, so any lookup would panic.
func TestListAtNoLookup(t *testing.T) {
	notFound := &azcore.ResponseError{StatusCode: 404}
	tests := []struct {
		name  string
		flags scopeFlags
		err   error
	}{
		{"ID that exists", scopeFlags{ManagementGroup: "mg-prod"}, nil},
		{"other error", scopeFlags{ManagementGroup: "mg-prod"}, errors.New("boom")},
		{"subscription scope", scopeFlags{Subscription: "s1"}, notFound},
		{"tenant root", scopeFlags{ManagementGroup: "/"}, notFound},
		{"explicit --scope", scopeFlags{Scope: "/x", ManagementGroup: "mg-prod"}, notFound},
	}
	for _, tt := range tests {
		calls := 0
		err := listAt(context.Background(), nil, tt.flags, "/scope", func(string) error { calls++; return tt.err })
		if calls != 1 || !errors.Is(err, tt.err) {
			t.Errorf("%s: calls = %d, err = %v; want 1 call and the list error", tt.name, calls, err)
		}
	}
}
