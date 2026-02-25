package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"azpim/internal/auth"
	"azpim/internal/pim"

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

If --role or --justification are omitted, interactive prompts will appear.

Examples:
  azpim extend                                                       # interactive, all scopes
  azpim extend --subscription <id> --role "Contributor" --duration 4h`,
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

		scopes, err := resolveScopes(ctx, clients, cred, extendScope)
		if err != nil {
			return err
		}

		active, err := clients.ListActiveForScopes(ctx, scopes, false)
		if err != nil {
			return fmt.Errorf("fetch active assignments: %w", err)
		}
		if len(active) == 0 {
			return fmt.Errorf("no active (time-bound) assignments found to extend")
		}

		selected, err := selectActive(active, extendRole)
		if err != nil {
			return err
		}

		maxDur := clients.FetchMaxActivationDuration(ctx, selected.Scope, selected.RoleDefID)

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

		principalID, err := auth.ResolvePrincipalID(ctx, cred)
		if err != nil {
			return fmt.Errorf("resolve principal ID: %w", err)
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
		if err := clients.Extend(ctx, opts); err != nil {
			return err
		}

		now := time.Now()
		if extendOutputFormat == "json" {
			return printExtendJSON(os.Stdout, selected.RoleName, selected.RoleDefID, selected.Scope, selected.Resource, dur, now)
		}
		fmt.Fprintf(os.Stdout, "✓ Role %q extended for %s at %q\n", selected.RoleName, pim.FormatDuration(dur), selected.Resource)
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
	ExtendedAt      string `json:"extended_at"`
	ExpiresAt       string `json:"expires_at"`
}

func printExtendJSON(w io.Writer, roleName, roleDefID, scope, scopeDisplay string, dur time.Duration, now time.Time) error {
	r := extendResult{
		RoleName:        roleName,
		RoleDefID:       roleDefID,
		Scope:           scope,
		ScopeDisplay:    scopeDisplay,
		Duration:        pim.FormatDuration(dur),
		DurationSeconds: int64(dur.Seconds()),
		ExtendedAt:      now.UTC().Format(time.RFC3339),
		ExpiresAt:       now.Add(dur).UTC().Format(time.RFC3339),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
