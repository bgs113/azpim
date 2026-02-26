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
	requestsScope   scopeFlags
	requestsOutput  outputFlags
	requestsPending bool
)

var requestsCmd = &cobra.Command{
	Use:   "requests",
	Short: "List your PIM role assignment requests",
	Long: `List role assignment schedule requests you have submitted.

Shows all requests by default (activate, extend, deactivate) with their
current status. Use --pending to show only requests awaiting admin approval.

Scope examples:
  azpim requests                                                    # all scopes (auto-discovered)
  azpim requests --pending                                          # only pending approval
  azpim requests --subscription 00000000-0000-0000-0000-000000000000`,
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

		scopes, err := resolveScopes(ctx, clients, cred, requestsScope)
		if err != nil {
			return err
		}

		requests, err := clients.ListRequestsForScopes(ctx, scopes)
		if err != nil {
			return err
		}

		switch requestsOutput.Format {
		case "json":
			return output.PrintRequestsJSON(os.Stdout, requests, requestsPending)
		case "table":
			output.PrintRequestsTable(os.Stdout, requests, requestsPending)
			return nil
		default:
			return fmt.Errorf("unknown output format %q (use table or json)", requestsOutput.Format)
		}
	},
}

func init() {
	addScopeFlags(requestsCmd, &requestsScope)
	addOutputFlags(requestsCmd, &requestsOutput)
	requestsCmd.Flags().BoolVar(&requestsPending, "pending", false, "Show only requests awaiting admin approval")
}
