// Package cmd implements azpim's Cobra commands: the scope, output and
// request flags they share, interactive prompts for missing values, and
// friendlier messages for Azure API errors.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/bgs113/azpim/internal/auth"
	"github.com/bgs113/azpim/internal/pim"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
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

// cloudName is the --cloud flag: which Azure cloud to sign in to and call.
var cloudName string

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
		if errors.Is(err, errCancelled) {
			os.Exit(130) // what the shell reports after Ctrl-C
		}
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
	case o.State() == "DryRun":
		return fmt.Sprintf("Dry run: %s request for %q not sent.", strings.ToLower(action), role)
	case o.State() == "Validated":
		return fmt.Sprintf("✓ %s request for %q passed Azure's validation. Nothing was submitted.", action, role)
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
	cloudDefault := os.Getenv("AZURE_CLOUD")
	if cloudDefault == "" {
		cloudDefault = "public"
	}
	rootCmd.PersistentFlags().StringVar(&cloudName, "cloud", cloudDefault, `Azure cloud: "public", "usgov" or "china" (env AZURE_CLOUD)`)
	rootCmd.AddCommand(eligibleCmd)
	rootCmd.AddCommand(activeCmd)
	rootCmd.AddCommand(activateCmd)
	rootCmd.AddCommand(deactivateCmd)
	rootCmd.AddCommand(extendCmd)
	rootCmd.AddCommand(requestsCmd)
}

// connect creates the credential and ARM clients for the --cloud cloud.
func connect() (*auth.Credential, *pim.Clients, error) {
	cred, err := auth.NewCredential(cloudName)
	if err != nil {
		return nil, nil, err
	}
	clients, err := pim.NewClients(cred, cred.Cloud)
	if err != nil {
		return nil, nil, err
	}
	return cred, clients, nil
}

// checkFlags are --dry-run and --validate-only, shared by the write commands.
type checkFlags struct {
	DryRun       bool
	ValidateOnly bool
}

func addCheckFlags(cmd *cobra.Command, f *checkFlags) {
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, "Show the request that would be sent, without sending anything")
	cmd.Flags().BoolVar(&f.ValidateOnly, "validate-only", false, "Have Azure validate the request against the role policy, without creating it")
	cmd.MarkFlagsMutuallyExclusive("dry-run", "validate-only")
}

// apply sets the clients' request mode and says on stderr when nothing will be submitted.
func (f checkFlags) apply(c *pim.Clients) {
	switch {
	case f.DryRun:
		c.Mode = pim.DryRun
		fmt.Fprintln(os.Stderr, "Dry run: nothing will be sent to Azure.")
	case f.ValidateOnly:
		c.Mode = pim.ValidateOnly
		fmt.Fprintln(os.Stderr, "Validate only: Azure checks the request but does not create it.")
	}
}

// addScopeFlags registers the shared scope flags on a command.
func addScopeFlags(cmd *cobra.Command, flags *scopeFlags) {
	cmd.Flags().StringVar(&flags.Scope, "scope", "", "Full ARM scope (overrides other scope flags)")
	cmd.Flags().StringVarP(&flags.ManagementGroup, "management-group", "m", "", `Management group ID or name (use "/" for tenant root group)`)
	cmd.Flags().StringVar(&flags.TenantID, "tenant-id", os.Getenv("AZURE_TENANT_ID"), "Azure tenant ID (auto-detected from credentials if omitted)")
	cmd.Flags().StringVarP(&flags.Subscription, "subscription", "s", "", "Subscription ID or name")
	cmd.Flags().StringVarP(&flags.ResourceGroup, "resource-group", "g", "", "Resource group name (requires --subscription)")
}

// resolveScope builds a single ARM scope string from flags.
// Used when an explicit scope is required (activate, deactivate).
func resolveScope(ctx context.Context, cred *auth.Credential, flags scopeFlags) (string, error) {
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
func queryScope(ctx context.Context, clients *pim.Clients, cred *auth.Credential, flags scopeFlags) (string, error) {
	if flags.Scope == "" && flags.ManagementGroup == "" && flags.Subscription == "" {
		return "/", nil
	}
	if flags.mgName() && !mgIDRe.MatchString(flags.ManagementGroup) {
		// Not a valid management group ID, so it can only be a display name.
		return clients.ResolveManagementGroup(ctx, flags.ManagementGroup)
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

// mgIDRe matches strings that are valid management group IDs: up to 90
// letters, digits, hyphens, underscores, periods and parentheses.
var mgIDRe = regexp.MustCompile(`^[\w.()-]{1,90}$`)

// mgName reports whether -m may name a management group by display name:
// it is set, is not the tenant root "/", and --scope does not override it.
func (f scopeFlags) mgName() bool {
	return f.Scope == "" && f.ManagementGroup != "" && f.ManagementGroup != "/"
}

// listAt runs list at scope. A display name that is also a valid ID, such as
// "Production", is first queried as an ID; if ARM reports that management
// group doesn't exist, listAt looks the name up and runs list once more at
// the group it names. IDs that exist cost no extra calls.
func listAt(ctx context.Context, clients *pim.Clients, flags scopeFlags, scope string, list func(scope string) error) error {
	err := list(scope)
	if err == nil || !flags.mgName() || !isNotFound(err) {
		return err
	}
	resolved, rerr := clients.ResolveManagementGroup(ctx, flags.ManagementGroup)
	if rerr != nil {
		return rerr
	}
	return list(resolved)
}

// isNotFound reports whether err is ARM saying the scope does not exist.
func isNotFound(err error) bool {
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return false
	}
	switch respErr.ErrorCode {
	case "InvalidScope", "ResourceNotFound", "ManagementGroupNotFound", "NotFound":
		return true
	}
	return respErr.StatusCode == http.StatusNotFound
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

// pickByLabel prompts the user to select one of items, using itemLabel to
// render each row; tabs in the label separate aligned columns. / filters the
// list. need names the flag that would have skipped the prompt, for the error
// when stdin isn't a terminal.
func pickByLabel[T any](items []T, label, need string, itemLabel func(T) string) (T, error) {
	labels := make([]string, len(items))
	for i, a := range items {
		labels[i] = itemLabel(a)
	}
	opts := make([]huh.Option[int], len(items))
	for i, l := range alignColumns(labels) {
		opts[i] = huh.NewOption(l, i)
	}
	var idx int
	sel := huh.NewSelect[int]().Title(label).Options(opts...).Value(&idx)
	// Scroll long lists in a fixed window so the title stays visible. Only for
	// long lists: huh misplaces a list shorter than its height as you move.
	if len(opts) > pickerRows {
		sel.Height(pickerRows + 1) // + the title
	}
	if err := runPrompt(sel, need); err != nil {
		var zero T
		return zero, err
	}
	echoAnswer(label, strings.ReplaceAll(labels[idx], "\t", "  "))
	return items[idx], nil
}

// pickerRows is how many rows a long picker list shows at once.
const pickerRows = 10

// alignColumns pads tab-separated rows so their columns line up.
func alignColumns(rows []string) []string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintln(w, r)
	}
	_ = w.Flush() // writes to a strings.Builder can't fail
	return strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
}

// scopeNeed is the need for pickByLabel when --role matches at several scopes.
const scopeNeed = "a scope flag (--subscription, --resource-group, --management-group or --scope)"

// stdinIsTerminal is a variable so tests can pretend there is no terminal.
var stdinIsTerminal = func() bool { return term.IsTerminal(os.Stdin.Fd()) }

// runPrompt shows field on stderr, keeping stdout clean for -o json. It fails
// without prompting when stdin isn't a terminal, naming need, the flag that
// would have skipped the prompt. Ctrl-C returns errCancelled.
// ACCESSIBLE=1 switches to huh's line-based mode for screen readers.
func runPrompt(field huh.Field, need string) error {
	if !stdinIsTerminal() {
		return fmt.Errorf("%s is required when not running in a terminal", need)
	}
	err := huh.NewForm(huh.NewGroup(field)).
		WithTheme(huh.ThemeCatppuccin()).
		WithOutput(os.Stderr).
		WithAccessible(plainPrompts()).
		Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return errCancelled
	}
	if err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	prompted = true
	return nil
}

// errCancelled means the user pressed Ctrl-C at a prompt. Execute exits
// without printing it.
var errCancelled = errors.New("cancelled")

// prompted is set once the user has answered a prompt, so write commands ask
// for confirmation only in interactive runs, never in flag-driven scripts.
var prompted bool

// confirm asks a yes/no question, preselecting def.
func confirm(question, need string, def bool) (bool, error) {
	ok := def
	c := huh.NewConfirm().Title(question).Affirmative("Yes").Negative("No").Value(&ok)
	if err := runPrompt(c, need); err != nil {
		return false, err
	}
	return ok, nil
}

// confirmRequest asks before sending a request built from prompted answers.
// It returns true without asking when nothing was prompted or the request
// won't be submitted (--dry-run, --validate-only).
func confirmRequest(c *pim.Clients, question string) (bool, error) {
	if !prompted || c.Mode != pim.Submit {
		return true, nil
	}
	ok, err := confirm(question, "--role", true)
	if err == nil && !ok {
		fmt.Fprintln(os.Stderr, "Nothing sent.")
	}
	return ok, err
}

// plainPrompts reports whether to use huh's line-based prompts, which print
// each question and read a typed answer, instead of the interactive UI.
func plainPrompts() bool { return os.Getenv("ACCESSIBLE") != "" }

// echoAnswer leaves "title: answer" on stderr, since huh clears the prompt
// once it's answered. Plain prompts already leave the typed answer on screen.
func echoAnswer(title, answer string) {
	if !plainPrompts() {
		fmt.Fprintf(os.Stderr, "%s: %s\n", title, strings.TrimSpace(answer))
	}
}

// resolvePrincipal returns the caller's object ID for write commands and
// prints, to stderr, the identity azpim is acting as. DefaultAzureCredential
// prefers AZURE_CLIENT_ID/SECRET over az login, so a leftover service principal
// secret would otherwise elevate silently as that principal.
func resolvePrincipal(ctx context.Context, cred *auth.Credential) (string, error) {
	principalID, err := auth.ResolvePrincipalID(ctx, cred)
	if err != nil {
		return "", fmt.Errorf("resolve principal ID: %w", err)
	}
	if who, err := auth.ResolveIdentity(ctx, cred); err == nil {
		fmt.Fprintf(os.Stderr, "Acting as %s\n", who)
	}
	return principalID, nil
}
