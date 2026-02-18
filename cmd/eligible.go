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
	eligibleScope  scopeFlags
	eligibleOutput outputFlags
)

var eligibleCmd = &cobra.Command{
	Use:   "eligible",
	Short: "List eligible PIM role assignments",
	Long: `List all PIM-eligible role assignments for the current principal.

Scope examples:
  azpim eligible                                                    # all scopes (auto-discovered)
  azpim eligible --subscription 00000000-0000-0000-0000-000000000000
  azpim eligible --management-group myMG
  azpim eligible --management-group /
  azpim eligible --subscription <id> --resource-group myRG`,
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

		scopes, err := resolveScopes(ctx, clients, cred, eligibleScope)
		if err != nil {
			return err
		}

		assignments, err := clients.ListEligibleForScopes(ctx, scopes)
		if err != nil {
			return err
		}

		switch eligibleOutput.Format {
		case "json":
			return output.PrintEligibleJSON(os.Stdout, assignments)
		case "table":
			output.PrintEligibleTable(os.Stdout, assignments)
			return nil
		default:
			return fmt.Errorf("unknown output format %q (use table or json)", eligibleOutput.Format)
		}
	},
}

func init() {
	addScopeFlags(eligibleCmd, &eligibleScope)
	addOutputFlags(eligibleCmd, &eligibleOutput)
}
