package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bgs113/azpim/internal/auth"
	"github.com/bgs113/azpim/internal/output"
	"github.com/bgs113/azpim/internal/pim"

	"github.com/BurntSushi/toml"
)

// preset is a named set of roles to activate together, read from presets.toml.
type preset struct {
	Duration string        `toml:"duration"`
	At       []presetEntry `toml:"at"`
}

// presetEntry is one scope and the roles to activate there. The scope keys
// match the scope flags.
type presetEntry struct {
	Subscription    string   `toml:"subscription"`
	ManagementGroup string   `toml:"management-group"`
	ResourceGroup   string   `toml:"resource-group"`
	Scope           string   `toml:"scope"`
	Roles           []string `toml:"roles"`
}

// flags returns the entry's scope as scope flags, so it resolves exactly like
// the command line.
func (e presetEntry) flags(tenantID string) scopeFlags {
	return scopeFlags{
		Scope:           e.Scope,
		ManagementGroup: e.ManagementGroup,
		TenantID:        tenantID,
		Subscription:    e.Subscription,
		ResourceGroup:   e.ResourceGroup,
	}
}

// presetsPath is where presets are read from: azpim/presets.toml under
// $XDG_CONFIG_HOME, or ~/.config when it's unset, on every OS but Windows,
// which keeps %AppData%. (os.UserConfigDir would pick ~/Library/Application
// Support on macOS, an odd home for a hand-edited CLI config.)
func presetsPath() (string, error) {
	var dir string
	var err error
	switch {
	case runtime.GOOS == "windows":
		dir, err = os.UserConfigDir()
	case os.Getenv("XDG_CONFIG_HOME") != "":
		dir = os.Getenv("XDG_CONFIG_HOME")
	default:
		dir, err = os.UserHomeDir()
		dir = filepath.Join(dir, ".config")
	}
	if err != nil {
		return "", fmt.Errorf("find the config directory for presets: %w", err)
	}
	return filepath.Join(dir, "azpim", "presets.toml"), nil
}

// loadPreset reads the preset called name from the TOML file at path and
// checks it. Keys azpim doesn't know are an error, so a typo isn't ignored.
func loadPreset(path, name string) (preset, error) {
	var presets map[string]preset
	md, err := toml.DecodeFile(path, &presets)
	if errors.Is(err, fs.ErrNotExist) {
		return preset{}, fmt.Errorf("no presets file at %s — see https://github.com/bgs113/azpim#presets", path)
	}
	if err != nil {
		return preset{}, fmt.Errorf("read presets: %w", err)
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		names := make([]string, len(keys))
		for i, k := range keys {
			names[i] = k.String()
		}
		return preset{}, fmt.Errorf("%s: unknown key(s): %s", path, strings.Join(names, ", "))
	}
	p, ok := presets[name]
	if !ok {
		names := slices.Sorted(maps.Keys(presets))
		return preset{}, fmt.Errorf("no preset %q in %s (presets: %s)", name, path, strings.Join(names, ", "))
	}
	if err := p.validate(); err != nil {
		return preset{}, fmt.Errorf("preset %q in %s: %w", name, path, err)
	}
	return p, nil
}

// validate checks what the TOML types can't: a valid duration, and a scope
// and at least one role in every entry.
func (p preset) validate() error {
	if p.Duration != "" {
		if err := validateDuration(0)(p.Duration); err != nil {
			return fmt.Errorf("duration %q: %w", p.Duration, err)
		}
	}
	if len(p.At) == 0 {
		return fmt.Errorf("no roles: add an [[<preset>.at]] entry")
	}
	for i, e := range p.At {
		n := 0
		for _, s := range []string{e.Subscription, e.ManagementGroup, e.Scope} {
			if s != "" {
				n++
			}
		}
		switch {
		case n == 0 && e.ResourceGroup != "":
			return fmt.Errorf("at entry %d: resource-group needs subscription", i+1)
		case n == 0:
			return fmt.Errorf("at entry %d: no scope: set one of subscription, management-group or scope", i+1)
		case n > 1:
			return fmt.Errorf("at entry %d: more than one scope: set only one of subscription, management-group or scope", i+1)
		case e.ResourceGroup != "" && e.Subscription == "":
			return fmt.Errorf("at entry %d: resource-group needs subscription", i+1)
		case len(e.Roles) == 0 || slices.Contains(e.Roles, ""):
			return fmt.Errorf("at entry %d: roles must list at least one role, with no empty names", i+1)
		}
	}
	return nil
}

// presetTarget is one role a preset will activate.
type presetTarget struct {
	pim.EligibleAssignment
	Duration time.Duration
}

// listFunc lists the eligible and active assignments for a preset entry, and
// returns the scope the entry resolved to. planPreset calls it for every
// entry at the same time.
type listFunc func(presetEntry) (scope string, eligible []pim.EligibleAssignment, active []pim.ActiveAssignment, err error)

// planPreset finds the eligible assignment for every role in p, looking up
// all entries at the same time. Roles that are already active are returned in
// skipped. It looks up every entry before returning, so the error lists every
// problem, not just the first.
func planPreset(p preset, list listFunc) (targets, skipped []pim.EligibleAssignment, err error) {
	type lookup struct {
		scope    string
		eligible []pim.EligibleAssignment
		active   []pim.ActiveAssignment
		err      error
	}
	lookups := make([]lookup, len(p.At))
	var wg sync.WaitGroup
	for i, e := range p.At {
		wg.Go(func() {
			l := &lookups[i]
			l.scope, l.eligible, l.active, l.err = list(e)
		})
	}
	wg.Wait()

	// Match in entry order, so problems and results don't depend on timing.
	var problems []string
	for i, e := range p.At {
		scope, eligible, active, err := lookups[i].scope, lookups[i].eligible, lookups[i].active, lookups[i].err
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %s", entryLabel(e), friendlyError(err)))
			continue
		}
		for _, role := range e.Roles {
			a, err := matchPresetRole(eligible, role, scope)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", entryLabel(e), err))
				continue
			}
			same := func(b pim.EligibleAssignment) bool { return b.RoleDefID == a.RoleDefID && b.Scope == a.Scope }
			switch {
			case slices.ContainsFunc(targets, same) || slices.ContainsFunc(skipped, same):
				// Listed twice; activating it twice would fail the second time.
			case len(pim.FilterEligibleActive([]pim.EligibleAssignment{a}, active)) == 0:
				skipped = append(skipped, a)
			default:
				targets = append(targets, a)
			}
		}
	}
	if len(problems) > 0 {
		return nil, nil, fmt.Errorf("nothing activated:\n  %s", strings.Join(problems, "\n  "))
	}
	return targets, skipped, nil
}

// entryLabel names a preset entry in messages by its scope key.
func entryLabel(e presetEntry) string {
	switch {
	case e.Scope != "":
		return "scope " + e.Scope
	case e.ManagementGroup != "":
		return "management group " + e.ManagementGroup
	case e.ResourceGroup != "":
		return "resource group " + e.Subscription + "/" + e.ResourceGroup
	}
	return "subscription " + e.Subscription
}

// matchPresetRole finds role among eligible like --role does. If it matches
// several assignments, the one at scope wins; with none there, it's ambiguous.
func matchPresetRole(eligible []pim.EligibleAssignment, role, scope string) (pim.EligibleAssignment, error) {
	matches := matchByRoleName(eligible, role, func(a pim.EligibleAssignment) string { return a.RoleName })
	if len(matches) > 1 {
		var here []pim.EligibleAssignment
		for _, m := range matches {
			if strings.EqualFold(strings.TrimSuffix(m.Scope, "/"), strings.TrimSuffix(scope, "/")) {
				here = append(here, m)
			}
		}
		if len(here) == 1 {
			return here[0], nil
		}
		labels := make([]string, len(matches))
		for i, m := range matches {
			labels[i] = fmt.Sprintf("%s at %s", m.RoleName, m.ScopeDisplay)
		}
		return pim.EligibleAssignment{}, fmt.Errorf("%q matches %d assignments (%s): use the full role name or a narrower scope", role, len(matches), strings.Join(labels, "; "))
	}
	if len(matches) == 0 {
		return pim.EligibleAssignment{}, fmt.Errorf("not eligible for %q", role)
	}
	return matches[0], nil
}

// activateTargets activates every target at the same time and returns one
// result per target, in order. The error says how many failed.
func activateTargets(targets []presetTarget, start time.Time, activate func(presetTarget) (pim.RequestOutcome, error)) ([]activateResult, error) {
	results := make([]activateResult, len(targets))
	var wg sync.WaitGroup
	now := time.Now()
	for i, t := range targets {
		wg.Go(func() {
			o, err := activate(t)
			results[i] = newActivateResult(t.RoleName, t.RoleDefID, t.Scope, t.ScopeDisplay, t.Duration, now, start, o)
			if err != nil {
				results[i].Status = "Failed"
				results[i].Error = friendlyError(err)
			}
		})
	}
	wg.Wait()
	failed := 0
	for _, r := range results {
		if r.Error != "" {
			failed++
		}
	}
	if failed > 0 {
		return results, fmt.Errorf("%d of %d role(s) failed to activate", failed, len(results))
	}
	return results, nil
}

// runPreset activates every role in the preset called name. It looks up all
// of them before activating any, and skips roles that are already active.
func runPreset(ctx context.Context, cred *auth.Credential, clients *pim.Clients, principalID, name string, p preset, start time.Time) error {
	durFlag := activateDuration
	if durFlag == "" {
		durFlag = p.Duration
	} else if err := validateDuration(0)(durFlag); err != nil {
		return fmt.Errorf("invalid duration %q: %w", durFlag, err)
	}

	var targets, skipped []pim.EligibleAssignment
	err := spin(ctx, "Looking up the preset's roles…", func(ctx context.Context) (err error) {
		targets, skipped, err = planPreset(p, func(e presetEntry) (scope string, eligible []pim.EligibleAssignment, active []pim.ActiveAssignment, err error) {
			flags := e.flags(activateScope.TenantID)
			if scope, err = queryScope(ctx, clients, cred, flags); err != nil {
				return "", nil, nil, err
			}
			err = listAt(ctx, clients, flags, scope, func(s string) (err error) {
				scope = s
				eligible, active, err = fetchAssignments(ctx, clients, s)
				return err
			})
			return scope, eligible, active, err
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("preset %q: %w", name, err)
	}
	for _, s := range skipped {
		fmt.Fprintf(os.Stderr, "Skipping %q at %q: already active\n", s.RoleName, s.ScopeDisplay)
	}

	// Each role gets the requested duration, capped at its policy maximum,
	// or the maximum when no duration is set.
	maxDurs := make([]time.Duration, len(targets))
	maxErrs := make([]error, len(targets))
	err = spin(ctx, "Reading the role policies…", func(ctx context.Context) error {
		var wg sync.WaitGroup
		for i, t := range targets {
			wg.Go(func() { maxDurs[i], maxErrs[i] = clients.FetchMaxActivationDuration(ctx, t.Scope, t.RoleDefID) })
		}
		wg.Wait()
		return ctx.Err()
	})
	if err != nil {
		return err
	}
	planned := make([]presetTarget, len(targets))
	for i, t := range targets {
		if maxErrs[i] != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch the policy maximum for %q — no duration cap will be enforced\n", t.RoleName)
		}
		planned[i] = presetTarget{t, presetDuration(durFlag, maxDurs[i])}
		if durFlag != "" && planned[i].Duration < mustParseDuration(durFlag) {
			fmt.Fprintf(os.Stderr, "%q at %q: capped at the policy maximum of %s\n", t.RoleName, t.ScopeDisplay, pim.FormatDuration(maxDurs[i]))
		}
	}

	var results []activateResult
	if len(planned) > 0 {
		justification := activateJustification
		if justification == "" {
			if justification, err = promptJustification(); err != nil {
				return err
			}
		}
		if ok, err := confirmRequest(clients, fmt.Sprintf("Activate %d role(s) from preset %q?", len(planned), name)); !ok {
			return err
		}
		fmt.Fprintf(os.Stderr, "Activating %d role(s)...\n", len(planned))
		results, err = activateTargets(planned, start, func(t presetTarget) (pim.RequestOutcome, error) {
			return clients.Activate(ctx, pim.ActivateOptions{
				RoleDefID:     t.RoleDefID,
				PrincipalID:   principalID,
				Scope:         t.Scope,
				Duration:      t.Duration,
				Start:         start,
				Justification: justification,
				TicketNumber:  activateTicketNumber,
				TicketSystem:  activateTicketSystem,
			})
		})
	}
	for _, s := range skipped {
		results = append(results, activateResult{RoleName: s.RoleName, RoleDefID: s.RoleDefID, Scope: s.Scope, ScopeDisplay: s.ScopeDisplay, Status: "AlreadyActive"})
	}

	if activateOutputFormat == "json" {
		if jerr := writeJSON(os.Stdout, results); jerr != nil {
			return jerr
		}
		return err
	}
	rows := make([][]string, len(results))
	for i, r := range results {
		rows[i] = []string{r.RoleName, r.ScopeDisplay, orDash(r.Duration), orDash(r.Status), orDash(r.Error)}
	}
	output.PrintActivationsTable(os.Stdout, rows)
	return err
}

// presetDuration is flag (already validated) capped at maxDur, or maxDur when
// flag is empty. With neither, it's an hour, the prompt's default.
func presetDuration(flag string, maxDur time.Duration) time.Duration {
	if flag == "" {
		if maxDur > 0 {
			return maxDur
		}
		return time.Hour
	}
	d := mustParseDuration(flag)
	if maxDur > 0 && d > maxDur {
		return maxDur
	}
	return d
}

// mustParseDuration parses a duration that validateDuration already accepted.
func mustParseDuration(s string) time.Duration {
	d, _ := parseDurationInput(s)
	return d
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
