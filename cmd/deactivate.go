package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"azpim/internal/auth"
	"azpim/internal/pim"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	deactivateScope        scopeFlags
	deactivateRole         string
	deactivateAll          bool
	deactivateYes          bool
	deactivateOutputFormat string
)

var deactivateCmd = &cobra.Command{
	Use:   "deactivate",
	Short: "Deactivate an active PIM role assignment",
	Long: `Deactivate an active (time-bound) PIM role assignment for the current principal.

If --role is omitted, an interactive list of active assignments is presented.

Examples:
  azpim deactivate                                           # interactive, all scopes
  azpim deactivate --subscription <id> --role "Contributor"
  azpim deactivate --all --yes                               # deactivate all without prompting`,
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

		scopes, err := resolveScopes(ctx, clients, cred, deactivateScope)
		if err != nil {
			return err
		}

		// Only time-bound (Activated) assignments can be deactivated.
		active, err := clients.ListActiveForScopes(ctx, scopes, false)
		if err != nil {
			return fmt.Errorf("fetch active assignments: %w", err)
		}
		if len(active) == 0 {
			return fmt.Errorf("no active (time-bound) assignments found")
		}

		principalID, err := auth.ResolvePrincipalID(ctx, cred)
		if err != nil {
			return fmt.Errorf("resolve principal ID: %w", err)
		}

		if deactivateAll {
			fmt.Fprintln(os.Stderr, "Active roles to be deactivated:")
			for _, a := range active {
				fmt.Fprintf(os.Stderr, "  • %-40s  %s  (%s remaining)\n", a.RoleName, a.Resource, a.TimeRemaining(false))
			}
			if !deactivateYes {
				fmt.Fprintf(os.Stderr, "Deactivate all %d role(s)? [y/N] ", len(active))
				var answer string
				fmt.Fscanln(os.Stdin, &answer) // #nosec G104
				if strings.ToLower(strings.TrimSpace(answer)) != "y" {
					fmt.Fprintln(os.Stderr, "No roles deactivated.")
					return nil
				}
			}
			now := time.Now()
			var deactivated []pim.ActiveAssignment
			var failed []string
			for _, a := range active {
				fmt.Fprintf(os.Stderr, "Deactivating %q...\n", a.RoleName)
				if err := clients.Deactivate(ctx, pim.DeactivateOptions{
					RoleDefID:   a.RoleDefID,
					PrincipalID: principalID,
					Scope:       a.Scope,
				}); err != nil {
					fmt.Fprintf(os.Stderr, "  error: %v\n", err)
					failed = append(failed, a.RoleName)
					continue
				}
				deactivated = append(deactivated, a)
				if deactivateOutputFormat != "json" {
					fmt.Fprintf(os.Stdout, "✓ Role %q deactivated\n", a.RoleName)
				}
			}
			if deactivateOutputFormat == "json" {
				if err := printDeactivateAllJSON(os.Stdout, deactivated, now); err != nil {
					return err
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("failed to deactivate: %s", strings.Join(failed, ", "))
			}
			return nil
		}

		selected, err := selectActive(active, deactivateRole)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "Deactivating %q...\n", selected.RoleName)
		if err := clients.Deactivate(ctx, pim.DeactivateOptions{
			RoleDefID:   selected.RoleDefID,
			PrincipalID: principalID,
			Scope:       selected.Scope,
		}); err != nil {
			return err
		}

		now := time.Now()
		if deactivateOutputFormat == "json" {
			return printDeactivateJSON(os.Stdout, selected.RoleName, selected.RoleDefID, selected.Scope, selected.Resource, now)
		}
		fmt.Fprintf(os.Stdout, "✓ Role %q deactivated successfully\n", selected.RoleName)
		return nil
	},
}

func init() {
	addScopeFlags(deactivateCmd, &deactivateScope)
	deactivateCmd.Flags().StringVar(&deactivateRole, "role", "", "Role name to deactivate (interactive if omitted)")
	deactivateCmd.Flags().BoolVar(&deactivateAll, "all", false, "Deactivate all active roles (prompts for confirmation unless --yes)")
	deactivateCmd.Flags().BoolVarP(&deactivateYes, "yes", "y", false, "Skip confirmation prompt when used with --all")
	deactivateCmd.Flags().StringVarP(&deactivateOutputFormat, "output", "o", "table", `Output format: "table" or "json"`)
}

// selectActive returns the matching active assignment. If roleFlag matches
// multiple assignments at different scopes, the user is prompted to disambiguate.
func selectActive(active []pim.ActiveAssignment, roleFlag string) (pim.ActiveAssignment, error) {
	if roleFlag != "" {
		matches := matchActive(active, roleFlag)
		if len(matches) == 0 {
			return pim.ActiveAssignment{}, fmt.Errorf("no active role matching %q found", roleFlag)
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		return pickActive(matches, fmt.Sprintf("Multiple %q assignments found — select scope", roleFlag))
	}
	return pickActive(active, "Select active role to deactivate")
}

func matchActive(active []pim.ActiveAssignment, roleFlag string) []pim.ActiveAssignment {
	// Exact match first.
	var exact []pim.ActiveAssignment
	for _, a := range active {
		if strings.EqualFold(a.RoleName, roleFlag) {
			exact = append(exact, a)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	// Prefix match fallback.
	var prefix []pim.ActiveAssignment
	for _, a := range active {
		if strings.HasPrefix(strings.ToLower(a.RoleName), strings.ToLower(roleFlag)) {
			prefix = append(prefix, a)
		}
	}
	return prefix
}

func pickActive(active []pim.ActiveAssignment, label string) (pim.ActiveAssignment, error) {
	labels := make([]string, len(active))
	for i, a := range active {
		labels[i] = fmt.Sprintf("%-40s  %s  (%s remaining)", a.RoleName, a.Resource, a.TimeRemaining(false))
	}
	prompt := promptui.Select{
		Label: label,
		Items: labels,
		Size:  15,
	}
	idx, _, err := prompt.Run()
	if err != nil {
		return pim.ActiveAssignment{}, fmt.Errorf("selection cancelled")
	}
	return active[idx], nil
}

type deactivateResult struct {
	RoleName      string `json:"role_name"`
	RoleDefID     string `json:"role_definition_id"`
	Scope         string `json:"scope"`
	ScopeDisplay  string `json:"scope_display"`
	DeactivatedAt string `json:"deactivated_at"`
}

func printDeactivateJSON(w io.Writer, roleName, roleDefID, scope, scopeDisplay string, now time.Time) error {
	r := deactivateResult{
		RoleName:      roleName,
		RoleDefID:     roleDefID,
		Scope:         scope,
		ScopeDisplay:  scopeDisplay,
		DeactivatedAt: now.UTC().Format(time.RFC3339),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func printDeactivateAllJSON(w io.Writer, roles []pim.ActiveAssignment, now time.Time) error {
	results := make([]deactivateResult, len(roles))
	for i, a := range roles {
		results[i] = deactivateResult{
			RoleName:      a.RoleName,
			RoleDefID:     a.RoleDefID,
			Scope:         a.Scope,
			ScopeDisplay:  a.Resource,
			DeactivatedAt: now.UTC().Format(time.RFC3339),
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}
