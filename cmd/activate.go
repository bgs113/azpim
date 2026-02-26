package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"azpim/internal/auth"
	"azpim/internal/pim"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	activateScope         scopeFlags
	activateRole          string
	activateDuration      string
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

Examples:
  azpim activate                                                              # interactive, all scopes
  azpim activate --subscription <id> --role "Contributor" --duration 2h --justification "incident response"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cred, err := auth.NewCredential()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		clients, err := pim.NewClients(cred)
		if err != nil {
			return err
		}
		defer clients.SaveRoleDefCache()

		scopes, err := resolveScopes(ctx, clients, cred, activateScope)
		if err != nil {
			return err
		}

		eligible, err := clients.ListEligibleForScopes(ctx, scopes)
		if err != nil {
			return fmt.Errorf("fetch eligible assignments: %w", err)
		}

		active, err := clients.ListActiveForScopes(ctx, scopes, false)
		if err != nil {
			return fmt.Errorf("fetch active assignments: %w", err)
		}
		eligible = pim.FilterEligibleActive(eligible, active)

		if len(eligible) == 0 {
			return fmt.Errorf("no eligible assignments available to activate (all may already be active)")
		}

		selected, err := selectEligible(eligible, activateRole)
		if err != nil {
			return err
		}

		maxDur := clients.FetchMaxActivationDuration(ctx, selected.Scope, selected.RoleDefID)

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

		principalID, err := auth.ResolvePrincipalID(ctx, cred)
		if err != nil {
			return fmt.Errorf("resolve principal ID: %w", err)
		}

		opts := pim.ActivateOptions{
			RoleDefID:     selected.RoleDefID,
			PrincipalID:   principalID,
			Scope:         selected.Scope,
			Duration:      dur,
			Justification: justification,
			TicketNumber:  activateTicketNumber,
			TicketSystem:  activateTicketSystem,
		}

		fmt.Fprintf(os.Stderr, "Activating %q for %s...\n", selected.RoleName, pim.FormatDuration(dur))
		outcome, err := clients.Activate(ctx, opts)
		if err != nil {
			return err
		}

		now := time.Now()
		if activateOutputFormat == "json" {
			return printActivateJSON(os.Stdout, selected.RoleName, selected.RoleDefID, selected.Scope, selected.ScopeDisplay, dur, now, outcome.Pending)
		}
		if outcome.Pending {
			fmt.Fprintf(os.Stdout, "Activation request submitted for %q — pending admin approval\n", selected.RoleName)
		} else {
			fmt.Fprintf(os.Stdout, "✓ Role %q activated for %s at %q\n", selected.RoleName, pim.FormatDuration(dur), selected.ScopeDisplay)
		}
		return nil
	},
}

func init() {
	addScopeFlags(activateCmd, &activateScope)
	activateCmd.Flags().StringVar(&activateRole, "role", "", "Role name to activate (interactive if omitted)")
	activateCmd.Flags().StringVarP(&activateDuration, "duration", "d", "", "Activation duration, e.g. 4 or 4h or 4h30m (prompts if omitted)")
	activateCmd.Flags().StringVarP(&activateJustification, "justification", "j", "", "Justification text (prompts if omitted)")
	activateCmd.Flags().StringVar(&activateTicketNumber, "ticket-number", "", "Ticket/incident number")
	activateCmd.Flags().StringVar(&activateTicketSystem, "ticket-system", "", "Ticket system URL")
	activateCmd.Flags().StringVarP(&activateOutputFormat, "output", "o", "table", `Output format: "table" or "json"`)
}

// selectEligible returns the matching eligible assignment. If roleFlag matches
// multiple assignments at different scopes, the user is prompted to disambiguate.
func selectEligible(eligible []pim.EligibleAssignment, roleFlag string) (pim.EligibleAssignment, error) {
	if roleFlag != "" {
		matches := matchEligible(eligible, roleFlag)
		if len(matches) == 0 {
			return pim.EligibleAssignment{}, fmt.Errorf("no eligible role matching %q found", roleFlag)
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		return pickEligible(matches, fmt.Sprintf("Multiple %q assignments found — select scope", roleFlag))
	}
	return pickEligible(eligible, "Select eligible role to activate")
}

func matchEligible(eligible []pim.EligibleAssignment, roleFlag string) []pim.EligibleAssignment {
	// Exact match first.
	var exact []pim.EligibleAssignment
	for _, a := range eligible {
		if strings.EqualFold(a.RoleName, roleFlag) {
			exact = append(exact, a)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	// Prefix match fallback.
	var prefix []pim.EligibleAssignment
	for _, a := range eligible {
		if strings.HasPrefix(strings.ToLower(a.RoleName), strings.ToLower(roleFlag)) {
			prefix = append(prefix, a)
		}
	}
	return prefix
}

func pickEligible(eligible []pim.EligibleAssignment, label string) (pim.EligibleAssignment, error) {
	labels := make([]string, len(eligible))
	for i, a := range eligible {
		labels[i] = fmt.Sprintf("%-40s  %s", a.RoleName, a.ScopeDisplay)
	}
	prompt := promptui.Select{
		Label: label,
		Items: labels,
		Size:  15,
	}
	idx, _, err := prompt.Run()
	if err != nil {
		return pim.EligibleAssignment{}, fmt.Errorf("selection cancelled")
	}
	return eligible[idx], nil
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
	ActivatedAt     string `json:"activated_at"`
	ExpiresAt       string `json:"expires_at"`
	Status          string `json:"status"`
}

func printActivateJSON(w io.Writer, roleName, roleDefID, scope, scopeDisplay string, dur time.Duration, now time.Time, pending bool) error {
	status := "Active"
	if pending {
		status = "PendingApproval"
	}
	r := activateResult{
		RoleName:        roleName,
		RoleDefID:       roleDefID,
		Scope:           scope,
		ScopeDisplay:    scopeDisplay,
		Duration:        pim.FormatDuration(dur),
		DurationSeconds: int64(dur.Seconds()),
		ActivatedAt:     now.UTC().Format(time.RFC3339),
		ExpiresAt:       now.Add(dur).UTC().Format(time.RFC3339),
		Status:          status,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
