package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/bgs113/azpim/internal/output"
	"github.com/bgs113/azpim/internal/pim"

	"github.com/spf13/cobra"
)

var (
	requestsScope   scopeFlags
	requestsOutput  outputFlags
	requestsPending bool
)

var requestsCmd = &cobra.Command{
	Use:   "requests",
	Args:  cobra.NoArgs,
	Short: "List your PIM role assignment requests",
	Long: `List role assignment schedule requests for you (your own activation requests,
plus any an admin made on your behalf).

Shows all requests by default (activate, extend, deactivate) with their
current status. Use --pending to show only requests awaiting admin approval.

Scope examples:
  azpim requests                                                    # whole tenant, one query
  azpim requests --pending                                          # only pending approval
  azpim requests --subscription 00000000-0000-0000-0000-000000000000

Sort examples (default: newest first):
  azpim requests --sort status,requested:desc                       # by status, newest first within each
  azpim requests --sort requested                                   # oldest first`,
	RunE: func(cmd *cobra.Command, args []string) error {
		terms, err := requestsSort.parse(requestsOutput.Sort, "requests")
		if err != nil {
			return err
		}
		cred, clients, err := connect()
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		var requests []pim.ScheduleRequestEntry
		err = spin(ctx, "Fetching requests…", func(ctx context.Context) error {
			scope, err := queryScope(ctx, clients, cred, requestsScope)
			if err != nil {
				return err
			}
			return listAt(ctx, clients, requestsScope, scope, func(scope string) (err error) {
				requests, err = clients.ListRequests(ctx, scope)
				return err
			})
		})
		if err != nil {
			return err
		}

		requestsSort.sort(requests, terms, requestsOutput.Reverse)
		switch requestsOutput.Format {
		case "json":
			return output.PrintRequestsJSON(os.Stdout, requests, requestsPending)
		case "table":
			output.PrintRequestsTable(os.Stdout, os.Stderr, requests, requestsPending)
			return nil
		default:
			return fmt.Errorf("unknown output format %q (use table or json)", requestsOutput.Format)
		}
	},
}

func init() {
	addScopeFlags(requestsCmd, &requestsScope)
	addOutputFlags(requestsCmd, &requestsOutput)
	addSortFlags(requestsCmd, &requestsOutput, requestsSort)
	requestsCmd.Flags().BoolVar(&requestsPending, "pending", false, "Show only requests awaiting admin approval")
}
