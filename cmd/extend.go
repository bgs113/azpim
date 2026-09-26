package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/bgs113/azpim/internal/pim"

	"github.com/spf13/cobra"
)

var (
	extendScope         scopeFlags
	extendRole          string
	extendDuration      string
	extendJustification string
	extendTicketNumber  string
	extendTicketSystem  string
	extendOutputFormat  string
)

var extendCmd = &cobra.Command{
	Use:   "extend",
	Short: "Extend an active PIM role assignment",
	Long: `Request an extension of an active (time-bound) PIM role assignment.

The extension sets a new duration from the current time. Whether the extension
is auto-approved or requires admin approval depends on the role's management policy.
If it needs approval, azpim says the request is pending and the end time does not
change until an approver acts; track it with 'azpim requests --pending'.

If --role or --justification are omitted, interactive prompts will appear.

Examples:
  azpim extend                                                       # interactive, all scopes
  azpim extend --subscription <id> --role "Contributor" --duration 4h`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cred, clients, err := connect()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		principalID, err := resolvePrincipal(ctx, cred)
		if err != nil {
			return err
		}

		scope, err := queryScope(ctx, clients, cred, extendScope)
		if err != nil {
			return err
		}

		active, err := clients.ListActive(ctx, scope, false)
		if err != nil {
			return fmt.Errorf("fetch active assignments: %w", err)
		}
		if len(active) == 0 {
			return fmt.Errorf("no active (time-bound) assignments found to extend")
		}

		selected, err := selectActive(active, extendRole, "extend")
		if err != nil {
			return err
		}

		maxDur, err := clients.FetchMaxActivationDuration(ctx, selected.Scope, selected.RoleDefID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch policy maximum duration — no duration cap will be enforced\n")
		}

		dur, err := resolveDuration(extendDuration, maxDur)
		if err != nil {
			return err
		}

		justification := extendJustification
		if justification == "" {
			justification, err = promptJustification()
			if err != nil {
				return err
			}
		}

		opts := pim.ExtendOptions{
			RoleDefID:     selected.RoleDefID,
			PrincipalID:   principalID,
			Scope:         selected.Scope,
			Duration:      dur,
			Justification: justification,
			TicketNumber:  extendTicketNumber,
			TicketSystem:  extendTicketSystem,
		}

		fmt.Fprintf(os.Stderr, "Extending %q for %s...\n", selected.RoleName, pim.FormatDuration(dur))
		outcome, err := clients.Extend(ctx, opts)
		if err != nil {
			return err
		}

		now := time.Now()
		if extendOutputFormat == "json" {
			return printExtendJSON(os.Stdout, selected.RoleName, selected.RoleDefID, selected.Scope, selected.Resource, dur, now, outcome)
		}
		fmt.Fprintln(os.Stdout, outcomeLine(outcome, "Extension", selected.RoleName,
			fmt.Sprintf("✓ Role %q extended for %s at %q", selected.RoleName, pim.FormatDuration(dur), selected.Resource)))
		return nil
	},
}

func init() {
	addScopeFlags(extendCmd, &extendScope)
	extendCmd.Flags().StringVar(&extendRole, "role", "", "Role name to extend (interactive if omitted)")
	extendCmd.Flags().StringVarP(&extendDuration, "duration", "d", "", "New duration from now, e.g. 4h or 4h30m (prompts if omitted)")
	extendCmd.Flags().StringVarP(&extendJustification, "justification", "j", "", "Justification text (prompts if omitted)")
	extendCmd.Flags().StringVar(&extendTicketNumber, "ticket-number", "", "Ticket/incident number")
	extendCmd.Flags().StringVar(&extendTicketSystem, "ticket-system", "", "Ticket system URL")
	extendCmd.Flags().StringVarP(&extendOutputFormat, "output", "o", "table", `Output format: "table" or "json"`)
}

type extendResult struct {
	RoleName        string `json:"role_name"`
	RoleDefID       string `json:"role_definition_id"`
	Scope           string `json:"scope"`
	ScopeDisplay    string `json:"scope_display"`
	Duration        string `json:"duration"`
	DurationSeconds int64  `json:"duration_seconds"`
	RequestedAt     string `json:"requested_at"`
	ExtendedAt      string `json:"extended_at,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	Status          string `json:"status"`
	AzureStatus     string `json:"azure_status"`
}

// printExtendJSON writes the result. extended_at and expires_at are only set
// once Azure confirms the extension took effect.
func printExtendJSON(w io.Writer, roleName, roleDefID, scope, scopeDisplay string, dur time.Duration, now time.Time, o pim.RequestOutcome) error {
	r := extendResult{
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
	if o.Done() {
		r.ExtendedAt = r.RequestedAt
		r.ExpiresAt = now.Add(dur).UTC().Format(time.RFC3339)
	}
	return writeJSON(w, r)
}
