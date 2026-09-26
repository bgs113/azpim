package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bgs113/azpim/internal/pim"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var (
	activateCheck         checkFlags
	activateScope         scopeFlags
	activateRole          string
	activateDuration      string
	activateStart         string
	activateJustification string
	activateTicketNumber  string
	activateTicketSystem  string
	activateOutputFormat  string
)

var activateCmd = &cobra.Command{
	Use:   "activate",
	Short: "Activate an eligible PIM role assignment",
	Long: `Activate an eligible PIM role assignment for the current principal.

If --role or --justification are omitted, interactive prompts will appear.

Duration: integer hours (e.g. 4), or duration string (e.g. 4h30m, 90m).
Defaults to the maximum allowed by the role policy.

Start: --start schedules the activation for later instead of now. Accepts
RFC 3339 (2026-10-01T22:00:00Z), a local date and time (2026-10-01T22:00), or
a local time (22:00: today, or tomorrow if that time has passed).

Examples:
  azpim activate                                                              # interactive, all scopes
  azpim activate --subscription <id> --role "Contributor" --duration 2h --justification "incident response"
  azpim activate --role "Contributor" -d 2h -j "test" --validate-only        # check against the role policy only
  azpim activate --subscription Prod --role Owner --start 22:00 -d 4h -j "CHG-5678"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cred, clients, err := connect()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		activateCheck.apply(clients)
		var start time.Time
		if activateStart != "" {
			if start, err = parseStart(activateStart, time.Now()); err != nil {
				return err
			}
		}

		principalID, err := resolvePrincipal(ctx, cred)
		if err != nil {
			return err
		}

		scope, err := queryScope(ctx, clients, cred, activateScope)
		if err != nil {
			return err
		}

		var (
			eligible []pim.EligibleAssignment
			active   []pim.ActiveAssignment
		)
		err = listAt(ctx, clients, activateScope, scope, func(scope string) error {
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				var err error
				eligible, err = clients.ListEligible(gctx, scope)
				if err != nil {
					return fmt.Errorf("fetch eligible assignments: %w", err)
				}
				return nil
			})
			g.Go(func() error {
				var err error
				active, err = clients.ListActive(gctx, scope, false)
				if err != nil {
					return fmt.Errorf("fetch active assignments: %w", err)
				}
				return nil
			})
			return g.Wait()
		})
		if err != nil {
			return err
		}
		eligible = pim.FilterEligibleActive(eligible, active)

		if len(eligible) == 0 {
			return fmt.Errorf("no eligible assignments available to activate (all may already be active)")
		}

		selected, err := selectEligible(eligible, activateRole)
		if err != nil {
			return err
		}

		maxDur, err := clients.FetchMaxActivationDuration(ctx, selected.Scope, selected.RoleDefID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch policy maximum duration — no duration cap will be enforced\n")
		}

		dur, err := resolveDuration(activateDuration, maxDur)
		if err != nil {
			return err
		}

		justification := activateJustification
		if justification == "" {
			justification, err = promptJustification()
			if err != nil {
				return err
			}
		}

		opts := pim.ActivateOptions{
			RoleDefID:     selected.RoleDefID,
			PrincipalID:   principalID,
			Scope:         selected.Scope,
			Duration:      dur,
			Start:         start,
			Justification: justification,
			TicketNumber:  activateTicketNumber,
			TicketSystem:  activateTicketSystem,
		}

		if start.IsZero() {
			fmt.Fprintf(os.Stderr, "Activating %q for %s...\n", selected.RoleName, pim.FormatDuration(dur))
		} else {
			fmt.Fprintf(os.Stderr, "Scheduling %q for %s from %s...\n", selected.RoleName, pim.FormatDuration(dur), start.Format(startLayout))
		}
		outcome, err := clients.Activate(ctx, opts)
		if err != nil {
			return err
		}

		now := time.Now()
		if activateOutputFormat == "json" {
			return printActivateJSON(os.Stdout, selected.RoleName, selected.RoleDefID, selected.Scope, selected.ScopeDisplay, dur, now, start, outcome)
		}
		if !start.IsZero() && outcome.Scheduled() {
			fmt.Fprintf(os.Stdout, "✓ Role %q scheduled for %s from %s at %q\n", selected.RoleName, pim.FormatDuration(dur), start.Format(startLayout), selected.ScopeDisplay)
			return nil
		}
		fmt.Fprintln(os.Stdout, outcomeLine(outcome, "Activation", selected.RoleName,
			fmt.Sprintf("✓ Role %q activated for %s at %q", selected.RoleName, pim.FormatDuration(dur), selected.ScopeDisplay)))
		return nil
	},
}

func init() {
	addScopeFlags(activateCmd, &activateScope)
	addCheckFlags(activateCmd, &activateCheck)
	activateCmd.Flags().StringVar(&activateRole, "role", "", "Role name to activate (interactive if omitted)")
	activateCmd.Flags().StringVarP(&activateDuration, "duration", "d", "", "Activation duration, e.g. 4 or 4h or 4h30m (prompts if omitted)")
	activateCmd.Flags().StringVar(&activateStart, "start", "", "Start the activation later: 22:00, 2026-10-01T22:00 (local time) or RFC 3339")
	activateCmd.Flags().StringVarP(&activateJustification, "justification", "j", "", "Justification text (prompts if omitted)")
	activateCmd.Flags().StringVar(&activateTicketNumber, "ticket-number", "", "Ticket/incident number")
	activateCmd.Flags().StringVar(&activateTicketSystem, "ticket-system", "", "Ticket system URL")
	activateCmd.Flags().StringVarP(&activateOutputFormat, "output", "o", "table", `Output format: "table" or "json"`)
}

// selectEligible returns the matching eligible assignment. If roleFlag matches
// multiple assignments at different scopes, the user is prompted to disambiguate.
func selectEligible(eligible []pim.EligibleAssignment, roleFlag string) (pim.EligibleAssignment, error) {
	if roleFlag != "" {
		matches := matchByRoleName(eligible, roleFlag, func(a pim.EligibleAssignment) string { return a.RoleName })
		if len(matches) == 0 {
			return pim.EligibleAssignment{}, fmt.Errorf("no eligible role matching %q found", roleFlag)
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		return pickByLabel(matches, fmt.Sprintf("Multiple %q assignments found — select scope", roleFlag), eligibleLabel)
	}
	return pickByLabel(eligible, "Select eligible role to activate", eligibleLabel)
}

func eligibleLabel(a pim.EligibleAssignment) string {
	return fmt.Sprintf("%-40s  %s", a.RoleName, a.ScopeDisplay)
}

// resolveDuration parses the --duration flag or prompts the user.
// Accepts an integer (hours) or a Go duration string (e.g. 4h30m, 90m).
func resolveDuration(flag string, maxDur time.Duration) (time.Duration, error) {
	if flag != "" {
		d, err := parseDurationInput(flag)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: use integer hours (e.g. 4) or duration string (e.g. 4h30m, 90m)", flag)
		}
		if d <= 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		if maxDur > 0 && d > maxDur {
			return 0, fmt.Errorf("duration exceeds maximum allowed by policy (%s)", pim.FormatDuration(maxDur))
		}
		return d, nil
	}

	defaultStr := "1h"
	if maxDur > 0 {
		defaultStr = pim.FormatDuration(maxDur)
	}

	prompt := promptui.Prompt{
		Label:   fmt.Sprintf("Duration [%s]", defaultStr),
		Default: defaultStr,
		Validate: func(s string) error {
			d, err := parseDurationInput(strings.TrimSpace(s))
			if err != nil {
				return fmt.Errorf("use integer hours (e.g. 4) or duration string (e.g. 4h30m, 90m)")
			}
			if d <= 0 {
				return fmt.Errorf("duration must be positive")
			}
			if maxDur > 0 && d > maxDur {
				return fmt.Errorf("exceeds maximum %s", pim.FormatDuration(maxDur))
			}
			return nil
		},
	}
	val, err := prompt.Run()
	if err != nil {
		return 0, fmt.Errorf("prompt cancelled")
	}
	d, err := parseDurationInput(strings.TrimSpace(val))
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", val, err)
	}
	return d, nil
}

// parseDurationInput accepts an integer (treated as hours) or a Go duration string.
func parseDurationInput(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if h, err := strconv.Atoi(s); err == nil {
		return time.Duration(h) * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// startLayout is how a scheduled start time is shown to the user.
const startLayout = "2006-01-02 15:04 MST"

// startGrace is how far in the past --start may be, to allow for clock skew.
const startGrace = 5 * time.Minute

// parseStart parses --start in now's location. It accepts RFC 3339, a date
// and time, or a time of day, which means the next time the clock shows it.
func parseStart(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	loc := now.Location()
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02T15:04", s, loc)
	}
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02 15:04", s, loc)
	}
	if err != nil {
		clock, cerr := time.ParseInLocation("15:04", s, loc)
		if cerr != nil {
			return time.Time{}, fmt.Errorf("invalid --start %q: use 22:00, 2026-10-01T22:00 or RFC 3339", s)
		}
		y, m, d := now.Date()
		// time.Date, not Add(24h), so a DST change overnight keeps the wall-clock time.
		t = time.Date(y, m, d, clock.Hour(), clock.Minute(), 0, 0, loc)
		if !t.After(now) {
			t = time.Date(y, m, d+1, clock.Hour(), clock.Minute(), 0, 0, loc)
		}
		return t, nil
	}
	if t.Before(now.Add(-startGrace)) {
		return time.Time{}, fmt.Errorf("--start %s is in the past", t.In(loc).Format(startLayout))
	}
	return t, nil
}

// promptJustification prompts interactively for a non-empty justification string.
func promptJustification() (string, error) {
	for {
		prompt := promptui.Prompt{Label: "Justification"}
		val, err := prompt.Run()
		if err != nil {
			return "", fmt.Errorf("prompt cancelled")
		}
		if strings.TrimSpace(val) != "" {
			return val, nil
		}
		fmt.Fprintln(os.Stderr, "  Justification is required")
	}
}

type activateResult struct {
	RoleName        string `json:"role_name"`
	RoleDefID       string `json:"role_definition_id"`
	Scope           string `json:"scope"`
	ScopeDisplay    string `json:"scope_display"`
	Duration        string `json:"duration"`
	DurationSeconds int64  `json:"duration_seconds"`
	RequestedAt     string `json:"requested_at"`
	ActivatedAt     string `json:"activated_at,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	Status          string `json:"status"`
	AzureStatus     string `json:"azure_status"`
}

// printActivateJSON writes the result. activated_at and expires_at are only
// set once Azure confirms the activation took effect or, for a scheduled
// activation (start is non-zero), is scheduled; they then count from start.
func printActivateJSON(w io.Writer, roleName, roleDefID, scope, scopeDisplay string, dur time.Duration, now, start time.Time, o pim.RequestOutcome) error {
	r := activateResult{
		RoleName:        roleName,
		RoleDefID:       roleDefID,
		Scope:           scope,
		ScopeDisplay:    scopeDisplay,
		Duration:        pim.FormatDuration(dur),
		DurationSeconds: int64(dur.Seconds()),
		RequestedAt:     now.UTC().Format(time.RFC3339),
		Status:          o.State(),
		AzureStatus:     o.Status,
	}
	switch {
	case !start.IsZero() && o.Scheduled():
		r.ActivatedAt = start.UTC().Format(time.RFC3339)
		r.ExpiresAt = start.Add(dur).UTC().Format(time.RFC3339)
	case o.Done():
		r.ActivatedAt = r.RequestedAt
		r.ExpiresAt = now.Add(dur).UTC().Format(time.RFC3339)
	}
	return writeJSON(w, r)
}
