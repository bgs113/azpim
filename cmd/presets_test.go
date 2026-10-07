package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bgs113/azpim/internal/pim"
)

func writePresets(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "presets.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPresetsPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses %AppData%")
	}
	t.Setenv("HOME", "/home/u")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got, _ := presetsPath(); got != "/home/u/.config/azpim/presets.toml" {
		t.Errorf("without XDG_CONFIG_HOME: %s", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got, _ := presetsPath(); got != "/xdg/azpim/presets.toml" {
		t.Errorf("with XDG_CONFIG_HOME: %s", got)
	}
}

func TestLoadPresetBothForms(t *testing.T) {
	want := preset{
		Duration: "4h",
		At: []presetEntry{
			{Subscription: "Prod", Roles: []string{"Contributor", "Reader"}},
			{Scope: "/subscriptions/x/vaults/kv", Roles: []string{"Key Vault Secrets Officer"}},
		},
	}
	tables := `
[prod-oncall]
duration = "4h"

[[prod-oncall.at]]
subscription = "Prod"
roles = ["Contributor", "Reader"]

[[prod-oncall.at]]
scope = "/subscriptions/x/vaults/kv"
roles = ["Key Vault Secrets Officer"]
`
	inline := `
[prod-oncall]
duration = "4h"
at = [
  { subscription = "Prod", roles = ["Contributor", "Reader"] },
  { scope = "/subscriptions/x/vaults/kv", roles = ["Key Vault Secrets Officer"] },
]
`
	for name, body := range map[string]string{"tables": tables, "inline": inline} {
		got, err := loadPreset(writePresets(t, body), "prod-oncall")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", name, got, want)
		}
	}
}

func TestLoadPresetErrors(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"unknown preset", "[a]\nat = [{ subscription = \"S\", roles = [\"R\"] }]\n", `no preset "p"`},
		{"unknown key", "[p]\nat = [{ subscription_id = \"S\", roles = [\"R\"] }]\n", "unknown key(s): p.at.subscription_id"},
		{"no scope", "[p]\nat = [{ roles = [\"R\"] }]\n", "no scope"},
		{"two scopes", "[p]\nat = [{ subscription = \"S\", scope = \"/x\", roles = [\"R\"] }]\n", "more than one scope"},
		{"resource group alone", "[p]\nat = [{ resource-group = \"rg\", roles = [\"R\"] }]\n", "resource-group needs subscription"},
		{"resource group with mg", "[p]\nat = [{ management-group = \"m\", resource-group = \"rg\", roles = [\"R\"] }]\n", "resource-group needs subscription"},
		{"empty roles", "[p]\nat = [{ subscription = \"S\", roles = [] }]\n", "at least one role"},
		{"no entries", "[p]\nduration = \"1h\"\n", "no roles"},
		{"bad duration", "[p]\nduration = \"soon\"\nat = [{ subscription = \"S\", roles = [\"R\"] }]\n", `duration "soon"`},
	}
	for _, tt := range tests {
		_, err := loadPreset(writePresets(t, tt.body), "p")
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want it to contain %q", tt.name, err, tt.want)
		}
	}

	_, err := loadPreset(filepath.Join(t.TempDir(), "missing.toml"), "p")
	if err == nil || !strings.Contains(err.Error(), "no presets file") {
		t.Errorf("missing file: err = %v", err)
	}
}

func TestPlanPresetReportsEveryProblem(t *testing.T) {
	contrib := pim.EligibleAssignment{RoleName: "Contributor", RoleDefID: "/r/contrib", Scope: "/subscriptions/a", ScopeDisplay: "A"}
	reader := pim.EligibleAssignment{RoleName: "Reader", RoleDefID: "/r/reader", Scope: "/subscriptions/a", ScopeDisplay: "A"}
	p := preset{At: []presetEntry{
		{Subscription: "A", Roles: []string{"Contributor", "Owner"}},
		{Subscription: "Missing", Roles: []string{"Reader"}},
		{Subscription: "A", Roles: []string{"Reader", "Contributor"}},
	}}
	list := func(e presetEntry) (string, []pim.EligibleAssignment, []pim.ActiveAssignment, error) {
		if e.Subscription == "Missing" {
			return "", nil, nil, errors.New(`no subscription found with name "Missing"`)
		}
		return "/subscriptions/a", []pim.EligibleAssignment{contrib, reader}, nil, nil
	}
	_, _, err := planPreset(p, list)
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{`not eligible for "Owner"`, `"Missing"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to mention %s", err, want)
		}
	}

	// Without the problems: Reader is active, so skipped; Contributor, listed
	// twice, is activated once.
	p.At = []presetEntry{p.At[0], p.At[2]}
	p.At[0].Roles = []string{"Contributor"}
	active := []pim.ActiveAssignment{{RoleName: "Reader", RoleDefID: "/r/reader", Scope: "/subscriptions/a", MembershipType: "Activated"}}
	targets, skipped, err := planPreset(p, func(presetEntry) (string, []pim.EligibleAssignment, []pim.ActiveAssignment, error) {
		return "/subscriptions/a", []pim.EligibleAssignment{contrib, reader}, active, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(targets, []pim.EligibleAssignment{contrib}) || !reflect.DeepEqual(skipped, []pim.EligibleAssignment{reader}) {
		t.Errorf("targets = %v, skipped = %v", targets, skipped)
	}
}

// Every entry's lookup starts before any finishes: each fake lookup waits
// for all of them to start, which deadlocks (and times out) if run in turn.
func TestPlanPresetLooksUpEntriesInParallel(t *testing.T) {
	p := preset{At: []presetEntry{
		{Subscription: "A", Roles: []string{"R"}},
		{Subscription: "B", Roles: []string{"R"}},
		{Subscription: "C", Roles: []string{"R"}},
	}}
	var started sync.WaitGroup
	started.Add(len(p.At))
	allStarted := make(chan struct{})
	go func() { started.Wait(); close(allStarted) }()
	_, _, err := planPreset(p, func(e presetEntry) (string, []pim.EligibleAssignment, []pim.ActiveAssignment, error) {
		started.Done()
		select {
		case <-allStarted:
		case <-time.After(5 * time.Second):
			return "", nil, nil, errors.New("lookups ran one at a time")
		}
		scope := "/subscriptions/" + e.Subscription
		return scope, []pim.EligibleAssignment{{RoleName: "R", RoleDefID: "/r", Scope: scope}}, nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMatchPresetRolePrefersScope(t *testing.T) {
	a := pim.EligibleAssignment{RoleName: "Contributor", Scope: "/subscriptions/a", ScopeDisplay: "A"}
	mg := pim.EligibleAssignment{RoleName: "Contributor", Scope: "/providers/Microsoft.Management/managementGroups/m", ScopeDisplay: "M"}
	got, err := matchPresetRole([]pim.EligibleAssignment{mg, a}, "contributor", "/subscriptions/A")
	if err != nil || got != a {
		t.Errorf("got %v, %v; want the assignment at the entry's scope", got, err)
	}
	if _, err := matchPresetRole([]pim.EligibleAssignment{mg, a}, "Contributor", "/subscriptions/b"); err == nil {
		t.Error("two matches, neither at the scope: want an ambiguity error")
	}
}

func TestActivateTargetsPartialFailure(t *testing.T) {
	targets := []presetTarget{
		{pim.EligibleAssignment{RoleName: "Contributor"}, time.Hour},
		{pim.EligibleAssignment{RoleName: "Owner"}, time.Hour},
		{pim.EligibleAssignment{RoleName: "Reader"}, time.Hour},
	}
	var calls atomic.Int32
	results, err := activateTargets(targets, time.Time{}, func(t presetTarget) (pim.RequestOutcome, error) {
		calls.Add(1)
		if t.RoleName == "Owner" {
			return pim.RequestOutcome{}, errors.New("boom")
		}
		return pim.RequestOutcome{Status: "Provisioned"}, nil
	})
	if calls.Load() != 3 {
		t.Errorf("activate called %d times, want 3 (one failure doesn't stop the rest)", calls.Load())
	}
	if err == nil || err.Error() != "1 of 3 role(s) failed to activate" {
		t.Errorf("err = %v", err)
	}
	var statuses []string
	for _, r := range results {
		statuses = append(statuses, r.Status)
	}
	if want := []string{"Active", "Failed", "Active"}; !reflect.DeepEqual(statuses, want) {
		t.Errorf("statuses = %v, want %v", statuses, want)
	}
	if results[1].Error != "boom" {
		t.Errorf("failed row error = %q", results[1].Error)
	}
}

func TestPresetDuration(t *testing.T) {
	tests := []struct {
		flag string
		max  time.Duration
		want time.Duration
	}{
		{"", 8 * time.Hour, 8 * time.Hour},
		{"", 0, time.Hour},
		{"4h", 8 * time.Hour, 4 * time.Hour},
		{"10", 8 * time.Hour, 8 * time.Hour}, // capped
		{"10", 0, 10 * time.Hour},
	}
	for _, tt := range tests {
		if got := presetDuration(tt.flag, tt.max); got != tt.want {
			t.Errorf("presetDuration(%q, %v) = %v, want %v", tt.flag, tt.max, got, tt.want)
		}
	}
}

func TestPresetExclusiveWithRole(t *testing.T) {
	t.Cleanup(func() {
		activatePreset, activateRole = "", ""
		_ = activateCmd.Flags().Set("preset", "")
		_ = activateCmd.Flags().Set("role", "")
		for _, f := range []string{"preset", "role"} {
			activateCmd.Flags().Lookup(f).Changed = false
		}
		rootCmd.SetArgs(nil)
	})
	rootCmd.SetArgs([]string{"activate", "--preset", "p", "--role", "Owner"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "none of the others can be") {
		t.Errorf("err = %v, want cobra's mutually exclusive flags error", err)
	}
}
