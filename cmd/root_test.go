package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bgs113/azpim/internal/pim"

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

	t.Run("timeout", func(t *testing.T) {
		err := fmt.Errorf("list eligible assignments at scope \"/\": %w", context.DeadlineExceeded)
		got := friendlyError(err)
		if !strings.HasPrefix(got, "Azure didn't respond") || !strings.Contains(got, pim.TryTimeout.String()) {
			t.Errorf("got %q, want the timeout message naming %s", got, pim.TryTimeout)
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

func TestAlignColumns(t *testing.T) {
	got := alignColumns([]string{
		"Azure Kubernetes Service RBAC Cluster Admin\tsub-prod",
		"Reader\tTenant Root Group",
	})
	want := []string{
		"Azure Kubernetes Service RBAC Cluster Admin  sub-prod",
		"Reader                                       Tenant Root Group",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("alignColumns =\n%q\nwant\n%q", got, want)
	}
}

func TestCommandsRejectPositionalArgs(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			if err := c.Args(c, []string{"extra"}); err == nil {
				t.Errorf("%s accepted an unexpected argument", c.Name())
			}
			if err := c.Args(c, nil); err != nil {
				t.Errorf("%s rejected no arguments: %v", c.Name(), err)
			}
		})
	}
}

func TestHumanFlagOnlyOnActive(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		has := c.Flags().Lookup("human") != nil
		if want := c == activeCmd; has != want {
			t.Errorf("%s: --human registered = %v, want %v", c.Name(), has, want)
		}
	}
}

// Without a terminal, spin just runs fn and passes its error through, drawing
// nothing (so piped and CI output stay clean).
func TestSpinWithoutTerminalRunsFn(t *testing.T) {
	origIn, origErr := stdinIsTerminal, stderrIsTerminal
	t.Cleanup(func() { stdinIsTerminal, stderrIsTerminal = origIn, origErr })
	stdinIsTerminal = func() bool { return true }
	stderrIsTerminal = func() bool { return false }

	boom := errors.New("boom")
	ran := false
	err := spin(context.Background(), "Working…", func(context.Context) error {
		ran = true
		return boom
	})
	if !ran || !errors.Is(err, boom) {
		t.Errorf("ran=%v err=%v, want fn run and its error returned", ran, err)
	}
}

func runRoot(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs(args)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetArgs(nil) })
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("azpim %v: %v", args, err)
	}
	return buf.String()
}

// Both ways to ask print one GNU-style line: "azpim <version>", the version
// being everything after the last space.
func TestVersionLine(t *testing.T) {
	orig := rootCmd.Version
	t.Cleanup(func() { rootCmd.Version = orig })
	rootCmd.Version = "1.2.3"
	for _, args := range [][]string{{"--version"}, {"version"}} {
		if got := runRoot(t, args...); got != "azpim 1.2.3\n" {
			t.Errorf("azpim %v = %q, want %q", args[0], got, "azpim 1.2.3\n")
		}
	}
}

func TestHelpEndsWithLinks(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"activate", "--help"}} {
		if got := runRoot(t, args...); !strings.HasSuffix(got, helpFooter) {
			t.Errorf("azpim %v help doesn't end with the links:\n%s", args, got)
		}
	}
}
