package cmd

import (
	"fmt"
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
)

var activateCmd = &cobra.Command{
	Use:   "activate",
	Short: "Activate an eligible PIM role assignment",
	Long: `Activate an eligible PIM role assignment for the current principal.

If --role or --justification are omitted, interactive prompts will appear.

Duration: integer hours, e.g. 4 (defaults to the maximum allowed by the role policy).

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
		for justification == "" {
			prompt := promptui.Prompt{Label: "Justification"}
			val, err := prompt.Run()
			if err != nil {
				return fmt.Errorf("prompt cancelled")
			}
			if strings.TrimSpace(val) == "" {
				fmt.Fprintln(os.Stderr, "  Justification is required")
				continue
			}
			justification = val
		}

		principalID, err := auth.ResolvePrincipalID(ctx, cred)
		if err != nil {
			return fmt.Errorf("resolve principal ID: %w", err)
		}

		opts := pim.ActivateOptions{
			RoleDefID:     selected.RoleDefID,
			PrincipalID:   principalID,
			Scope:         selected.Scope, // activate at the assignment's own scope
			Duration:      dur,
			Justification: justification,
			TicketNumber:  activateTicketNumber,
			TicketSystem:  activateTicketSystem,
		}

		fmt.Fprintf(os.Stderr, "Activating %q for %s...\n", selected.RoleName, pim.FormatDuration(dur))
		if err := clients.Activate(ctx, opts); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "✓ Role %q activated for %s at %q\n", selected.RoleName, pim.FormatDuration(dur), selected.ScopeDisplay)
		return nil
	},
}

func init() {
	addScopeFlags(activateCmd, &activateScope)
	activateCmd.Flags().StringVar(&activateRole, "role", "", "Role name to activate (interactive if omitted)")
	activateCmd.Flags().StringVarP(&activateDuration, "duration", "d", "", "Activation duration in hours, e.g. 4 or 4h (prompts if omitted)")
	activateCmd.Flags().StringVarP(&activateJustification, "justification", "j", "", "Justification text (prompts if omitted)")
	activateCmd.Flags().StringVar(&activateTicketNumber, "ticket-number", "", "Ticket/incident number")
	activateCmd.Flags().StringVar(&activateTicketSystem, "ticket-system", "", "Ticket system URL")
}

// selectEligible returns the matching eligible assignment, prompting the user
// to choose one interactively if roleFlag is empty or matches multiple.
func selectEligible(eligible []pim.EligibleAssignment, roleFlag string) (pim.EligibleAssignment, error) {
	if roleFlag != "" {
		for _, a := range eligible {
			if strings.EqualFold(a.RoleName, roleFlag) {
				return a, nil
			}
		}
		for _, a := range eligible {
			if strings.HasPrefix(strings.ToLower(a.RoleName), strings.ToLower(roleFlag)) {
				return a, nil
			}
		}
		return pim.EligibleAssignment{}, fmt.Errorf("no eligible role matching %q found", roleFlag)
	}

	labels := make([]string, len(eligible))
	for i, a := range eligible {
		labels[i] = fmt.Sprintf("%-40s  %s", a.RoleName, a.ScopeDisplay)
	}

	prompt := promptui.Select{
		Label: "Select eligible role to activate",
		Items: labels,
		Size:  15,
	}
	idx, _, err := prompt.Run()
	if err != nil {
		return pim.EligibleAssignment{}, fmt.Errorf("selection cancelled")
	}
	return eligible[idx], nil
}

// resolveDuration parses the --duration flag or prompts the user for an integer
// number of hours. maxDur is the policy-enforced maximum; 0 means unknown.
func resolveDuration(flag string, maxDur time.Duration) (time.Duration, error) {
	maxHours := int(maxDur.Hours())

	if flag != "" {
		// Accept plain integer (hours) or Go duration string.
		var d time.Duration
		if h, err := strconv.Atoi(strings.TrimSpace(flag)); err == nil {
			d = time.Duration(h) * time.Hour
		} else if parsed, err := time.ParseDuration(flag); err == nil {
			d = parsed
		} else {
			return 0, fmt.Errorf("invalid duration %q: use hours as integer (e.g. 4) or Go duration (e.g. 4h)", flag)
		}
		if d <= 0 {
			return 0, fmt.Errorf("duration must be positive")
		}
		if maxHours > 0 && d > time.Duration(maxHours)*time.Hour {
			return 0, fmt.Errorf("duration exceeds maximum allowed by policy (%dh)", maxHours)
		}
		return d, nil
	}

	defaultStr := "1"
	if maxHours > 0 {
		defaultStr = strconv.Itoa(maxHours)
	}

	prompt := promptui.Prompt{
		Label:   fmt.Sprintf("Duration (hours) [%s]", defaultStr),
		Default: defaultStr,
		Validate: func(s string) error {
			h, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil || h <= 0 {
				return fmt.Errorf("enter a positive whole number of hours")
			}
			if maxHours > 0 && h > maxHours {
				return fmt.Errorf("exceeds maximum %dh", maxHours)
			}
			return nil
		},
	}
	val, err := prompt.Run()
	if err != nil {
		return 0, fmt.Errorf("prompt cancelled")
	}
	h, _ := strconv.Atoi(strings.TrimSpace(val))
	return time.Duration(h) * time.Hour, nil
}
