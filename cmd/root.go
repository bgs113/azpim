package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bgs113/azpim/internal/auth"
	"github.com/bgs113/azpim/internal/pim"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/manifoldco/promptui"
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

// outcomeLine returns the line to print after submitting a request: done if
// Azure confirmed the change took effect, otherwise a line saying it has not.
func outcomeLine(o pim.RequestOutcome, action, role, done string) string {
	switch {
	case o.Done():
		return done
	case o.Pending():
		return fmt.Sprintf("%s request for %q submitted and pending (Azure status: %s). It is not in effect yet — see 'azpim requests --pending'.", action, role, o.Status)
	}
	status := o.Status
	if status == "" {
		status = "none"
	}
	return fmt.Sprintf("%s request for %q submitted, but Azure did not confirm it took effect (status: %s). Check 'azpim requests' before relying on it.", action, role, status)
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
	case "RoleAssignmentDoesNotExist":
		return "no active assignment for this role at this scope — Azure rejects changes for about 5 minutes after activation, so if you just activated it, wait and retry"
	case "PendingRoleAssignmentRequest":
		return "a request for this role is already pending — see 'azpim requests --pending'"
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
	rootCmd.AddCommand(extendCmd)
	rootCmd.AddCommand(requestsCmd)
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

// queryScope returns the ARM scope to list assignments at: the scope named by
// the flags, or "/" (the whole tenant, in one query) when none are set.
func queryScope(ctx context.Context, clients *pim.Clients, cred azcore.TokenCredential, flags scopeFlags) (string, error) {
	if flags.Scope == "" && flags.ManagementGroup == "" && flags.Subscription == "" {
		return "/", nil
	}
	if flags.Subscription != "" {
		id, err := clients.ResolveSubscriptionID(ctx, flags.Subscription)
		if err != nil {
			return "", err
		}
		flags.Subscription = id
	}
	return resolveScope(ctx, cred, flags)
}

// addOutputFlags registers output format flags on a command.
func addOutputFlags(cmd *cobra.Command, flags *outputFlags) {
	cmd.Flags().StringVarP(&flags.Format, "output", "o", "table", `Output format: "table" or "json"`)
	cmd.Flags().BoolVar(&flags.HumanReadable, "human", false, `Use human-readable time remaining format (e.g. "1h 32m 5s")`)
}

// matchByRoleName returns items whose name exactly matches roleFlag
// (case-insensitive), falling back to a case-insensitive prefix match.
func matchByRoleName[T any](items []T, roleFlag string, name func(T) string) []T {
	var exact []T
	for _, a := range items {
		if strings.EqualFold(name(a), roleFlag) {
			exact = append(exact, a)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	var prefix []T
	for _, a := range items {
		if strings.HasPrefix(strings.ToLower(name(a)), strings.ToLower(roleFlag)) {
			prefix = append(prefix, a)
		}
	}
	return prefix
}

// pickByLabel prompts the user to select one of items via promptui, using
// itemLabel to render each row.
func pickByLabel[T any](items []T, label string, itemLabel func(T) string) (T, error) {
	labels := make([]string, len(items))
	for i, a := range items {
		labels[i] = itemLabel(a)
	}
	prompt := promptui.Select{
		Label: label,
		Items: labels,
		Size:  15,
	}
	idx, _, err := prompt.Run()
	if err != nil {
		var zero T
		return zero, fmt.Errorf("selection cancelled")
	}
	return items[idx], nil
}
