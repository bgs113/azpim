package cmd

import (
	"fmt"
	"os"
	"strings"

	"azpim/internal/auth"
	"azpim/internal/pim"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	deactivateScope scopeFlags
	deactivateRole  string
	deactivateAll   bool
)

var deactivateCmd = &cobra.Command{
	Use:   "deactivate",
	Short: "Deactivate an active PIM role assignment",
	Long: `Deactivate an active (time-bound) PIM role assignment for the current principal.

If --role is omitted, an interactive list of active assignments is presented.

Examples:
  azpim deactivate                                           # interactive, all scopes
  azpim deactivate --subscription <id> --role "Contributor"`,
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
			fmt.Fprintln(os.Stdout, "Active roles to be deactivated:")
			for _, a := range active {
				fmt.Fprintf(os.Stdout, "  • %-40s  %s  (%s remaining)\n", a.RoleName, a.Resource, a.TimeRemaining(false))
			}
			fmt.Fprintf(os.Stdout, "Deactivate all %d role(s)? [y/N] ", len(active))
			var answer string
			fmt.Fscanln(os.Stdin, &answer) // #nosec G104 -- EOF/error leaves answer empty, handled by != "y" check below
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				fmt.Fprintln(os.Stdout, "No roles deactivated.")
				return nil
			}
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
				fmt.Fprintf(os.Stdout, "✓ Role %q deactivated\n", a.RoleName)
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
		fmt.Fprintf(os.Stdout, "✓ Role %q deactivated successfully\n", selected.RoleName)
		return nil
	},
}

func init() {
	addScopeFlags(deactivateCmd, &deactivateScope)
	deactivateCmd.Flags().StringVar(&deactivateRole, "role", "", "Role name to deactivate (interactive if omitted)")
	deactivateCmd.Flags().BoolVar(&deactivateAll, "all", false, "Deactivate all active roles (prompts for confirmation)")
}

// selectActive returns the matching active assignment, prompting interactively
// if roleFlag is empty.
func selectActive(active []pim.ActiveAssignment, roleFlag string) (pim.ActiveAssignment, error) {
	if roleFlag != "" {
		for _, a := range active {
			if strings.EqualFold(a.RoleName, roleFlag) {
				return a, nil
			}
		}
		for _, a := range active {
			if strings.HasPrefix(strings.ToLower(a.RoleName), strings.ToLower(roleFlag)) {
				return a, nil
			}
		}
		return pim.ActiveAssignment{}, fmt.Errorf("no active role matching %q found", roleFlag)
	}

	labels := make([]string, len(active))
	for i, a := range active {
		labels[i] = fmt.Sprintf("%-40s  %s  (%s remaining)", a.RoleName, a.Resource, a.TimeRemaining(false))
	}

	prompt := promptui.Select{
		Label: "Select active role to deactivate",
		Items: labels,
		Size:  15,
	}
	idx, _, err := prompt.Run()
	if err != nil {
		return pim.ActiveAssignment{}, fmt.Errorf("selection cancelled")
	}
	return active[idx], nil
}
