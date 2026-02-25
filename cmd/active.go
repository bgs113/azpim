package cmd

import (
	"fmt"
	"os"

	"azpim/internal/auth"
	"azpim/internal/output"
	"azpim/internal/pim"

	"github.com/spf13/cobra"
)

var (
	activeScope            scopeFlags
	activeOutput           outputFlags
	activeIncludePermanent bool
)

var activeCmd = &cobra.Command{
	Use:   "active",
	Short: "List active PIM role assignments",
	Long: `List active (time-bound) PIM role assignments for the current principal.

By default, permanently-assigned roles are not shown. Use --include-permanent to
include them (they appear with "Permanent" in the Time remaining column).

Scope examples:
  azpim active                                                      # all scopes (auto-discovered)
  azpim active --subscription 00000000-0000-0000-0000-000000000000
  azpim active --management-group myMG --include-permanent`,
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

		scopes, err := resolveScopes(ctx, clients, cred, activeScope)
		if err != nil {
			return err
		}

		assignments, err := clients.ListActiveForScopes(ctx, scopes, activeIncludePermanent)
		if err != nil {
			return err
		}

		switch activeOutput.Format {
		case "json":
			return output.PrintActiveJSON(os.Stdout, assignments, activeOutput.HumanReadable)
		case "table":
			output.PrintActiveTable(os.Stdout, assignments, activeOutput.HumanReadable)
			return nil
		default:
			return fmt.Errorf("unknown output format %q (use table or json)", activeOutput.Format)
		}
	},
}

func init() {
	addScopeFlags(activeCmd, &activeScope)
	addOutputFlags(activeCmd, &activeOutput)
	activeCmd.Flags().BoolVar(&activeIncludePermanent, "include-permanent", false, "Include permanently-assigned roles (not shown by default)")
}
