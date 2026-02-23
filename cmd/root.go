package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"azpim/internal/auth"
	"azpim/internal/pim"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/spf13/cobra"
)

// scopeFlags are global flags for specifying the ARM scope, shared across subcommands.
type scopeFlags struct {
	Scope           string
	ManagementGroup string
	TenantID        string
	Subscription    string
	ResourceGroup   string
}

// outputFlags holds output format flags.
type outputFlags struct {
	Format        string // "table" or "json"
	HumanReadable bool   // --human for "1h 32m" style time remaining
}

var rootCmd = &cobra.Command{
	Use:   "azpim",
	Short: "Manage Azure PIM (Privileged Identity Management) sessions",
	Long: `azpim is a CLI tool for managing Azure Resource RBAC PIM sessions.

It supports listing eligible and active role assignments, and activating or
deactivating roles at management group, subscription, or resource group scope.

Authentication uses DefaultAzureCredential — run 'az login' before using this tool.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute is the entry point called from main.
func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", friendlyError(err))
		os.Exit(1)
	}
}

// friendlyError returns a concise, user-readable version of err.
// Auth errors from DefaultAzureCredential are collapsed to a one-liner, and
// verbose Azure ARM API HTTP dumps are trimmed to just the error code.
func friendlyError(err error) string {
	msg := err.Error()

	// Auth failures from the Azure SDK credential chain.
	if strings.Contains(msg, "DefaultAzureCredential") {
		if strings.Contains(msg, "expired") || strings.Contains(msg, "AADSTS50173") {
			return "Azure credentials have expired — run 'az login' to re-authenticate"
		}
		return "Azure authentication failed — run 'az login' to sign in"
	}

	// ARM API errors: strip the verbose multi-line HTTP dump and show one line.
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		// The SDK appends ": {METHOD} https://..." before the dump. Strip from there.
		for _, method := range []string{"GET", "PUT", "POST", "DELETE", "PATCH"} {
			if i := strings.Index(msg, ": "+method+" https://"); i >= 0 {
				msg = msg[:i]
				break
			}
		}
		return msg + ": " + armCodeDesc(respErr.ErrorCode, respErr.StatusCode)
	}

	return msg
}

// armCodeDesc maps known ARM error codes to short descriptions,
// falling back to "{code} (HTTP {status})" for unrecognised codes.
func armCodeDesc(code string, status int) string {
	switch code {
	case "RoleAssignmentExists", "RoleAssignmentAlreadyExists":
		return "role is already active"
	case "RoleAssignmentScheduleRequestExists":
		return "an activation request for this role is already pending"
	case "RoleEligibilityDoesNotExist":
		return "no eligible assignment found at this scope"
	case "PendingApproval", "PendingAdminDecision":
		return "role requires admin approval before it can be activated"
	case "AuthorizationFailed":
		return "permission denied"
	case "InvalidScope":
		return "invalid scope"
	}
	if code != "" {
		return fmt.Sprintf("%s (HTTP %d)", code, status)
	}
	return fmt.Sprintf("HTTP %d", status)
}

func init() {
	rootCmd.AddCommand(eligibleCmd)
	rootCmd.AddCommand(activeCmd)
	rootCmd.AddCommand(activateCmd)
	rootCmd.AddCommand(deactivateCmd)
}

// addScopeFlags registers the shared scope flags on a command.
func addScopeFlags(cmd *cobra.Command, flags *scopeFlags) {
	cmd.Flags().StringVar(&flags.Scope, "scope", "", "Full ARM scope (overrides other scope flags)")
	cmd.Flags().StringVarP(&flags.ManagementGroup, "management-group", "m", "", `Management group ID (use "/" for tenant root group)`)
	cmd.Flags().StringVar(&flags.TenantID, "tenant-id", os.Getenv("AZURE_TENANT_ID"), "Azure tenant ID (auto-detected from credentials if omitted)")
	cmd.Flags().StringVarP(&flags.Subscription, "subscription", "s", os.Getenv("AZURE_SUBSCRIPTION_ID"), "Subscription ID or name")
	cmd.Flags().StringVarP(&flags.ResourceGroup, "resource-group", "g", "", "Resource group name (requires --subscription)")
}

// resolveScope builds a single ARM scope string from flags.
// Used when an explicit scope is required (activate, deactivate).
func resolveScope(ctx context.Context, cred azcore.TokenCredential, flags scopeFlags) (string, error) {
	tenantID := flags.TenantID
	if flags.ManagementGroup == "/" && tenantID == "" {
		var err error
		tenantID, err = auth.ResolveTenantID(ctx, cred)
		if err != nil {
			return "", fmt.Errorf("could not auto-detect tenant ID from credentials (use --tenant-id): %w", err)
		}
	}
	return pim.BuildScope(flags.ManagementGroup, tenantID, flags.Subscription, flags.ResourceGroup, flags.Scope)
}

// resolveScopes returns the ARM scopes to query.
//
//   - With any explicit scope flag: resolves to a single-element slice.
//   - With no flags: enumerates all accessible subscriptions and management
//     groups in parallel, matching the Azure Portal "My roles" behavior.
func resolveScopes(ctx context.Context, clients *pim.Clients, cred azcore.TokenCredential, flags scopeFlags) ([]string, error) {
	if flags.Scope != "" || flags.ManagementGroup != "" || flags.Subscription != "" {
		if flags.Subscription != "" {
			id, err := clients.ResolveSubscriptionID(ctx, flags.Subscription)
			if err != nil {
				return nil, err
			}
			flags.Subscription = id
		}
		scope, err := resolveScope(ctx, cred, flags)
		if err != nil {
			return nil, err
		}
		return []string{scope}, nil
	}
	return clients.ListAccessibleScopes(ctx)
}

// addOutputFlags registers output format flags on a command.
func addOutputFlags(cmd *cobra.Command, flags *outputFlags) {
	cmd.Flags().StringVarP(&flags.Format, "output", "o", "table", `Output format: "table" or "json"`)
	cmd.Flags().BoolVar(&flags.HumanReadable, "human", false, `Use human-readable time remaining format (e.g. "1h 32m 5s")`)
}
