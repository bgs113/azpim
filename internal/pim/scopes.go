package pim

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/managementgroups/armmanagementgroups"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

const scopeCacheTTL = 5 * time.Minute

type scopeCacheFile struct {
	FetchedAt time.Time        `json:"fetched_at"`
	Entries   []scopeEntryJSON `json:"entries"`
}

type scopeEntryJSON struct {
	Scope       string `json:"scope"`
	DisplayName string `json:"display_name"`
}

func scopeCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "azpim", "scopes.json"), nil
}

func loadScopeCache() ([]scopeEntry, bool) {
	path, err := scopeCachePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cf scopeCacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, false
	}
	if time.Since(cf.FetchedAt) > scopeCacheTTL {
		return nil, false
	}
	entries := make([]scopeEntry, len(cf.Entries))
	for i, e := range cf.Entries {
		entries[i] = scopeEntry{scope: e.Scope, displayName: e.DisplayName}
	}
	return entries, true
}

func saveScopeCache(entries []scopeEntry) {
	path, err := scopeCachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	je := make([]scopeEntryJSON, len(entries))
	for i, e := range entries {
		je[i] = scopeEntryJSON{Scope: e.scope, DisplayName: e.displayName}
	}
	data, err := json.Marshal(scopeCacheFile{FetchedAt: time.Now(), Entries: je})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}

var subscriptionIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ResolveSubscriptionID returns a subscription ID for the given value.
// If nameOrID already looks like a UUID it is returned unchanged.
// Otherwise all accessible subscriptions are listed and the first one whose
// display name matches (case-insensitive) is returned.
func (c *Clients) ResolveSubscriptionID(ctx context.Context, nameOrID string) (string, error) {
	if subscriptionIDRe.MatchString(nameOrID) {
		return nameOrID, nil
	}
	entries, err := c.listSubscriptionEntries(ctx)
	if err != nil {
		return "", fmt.Errorf("list subscriptions: %w", err)
	}
	lower := strings.ToLower(nameOrID)
	for _, e := range entries {
		if strings.ToLower(e.displayName) == lower {
			return strings.TrimPrefix(e.scope, "/subscriptions/"), nil
		}
	}
	return "", fmt.Errorf("no subscription found with name %q", nameOrID)
}

// scopeEntry pairs an ARM scope string with its human-readable display name.
type scopeEntry struct {
	scope       string
	displayName string
}

// ListAccessibleScopes returns ARM scope strings for all subscriptions and
// management groups the current principal can enumerate. Results are cached on
// disk for 5 minutes to avoid redundant API calls on every invocation.
//
// Management group listing is best-effort: failures (e.g. 403) are silently
// ignored, since many orgs restrict that endpoint but users still have
// subscription-level PIM roles.
//
// Display names discovered during enumeration are stored in scopeNameCache so
// that subsequent calls to ResolveScopeName need not make extra API calls.
func (c *Clients) ListAccessibleScopes(ctx context.Context) ([]string, error) {
	// Serve from disk cache if fresh.
	if cached, ok := loadScopeCache(); ok {
		for _, e := range cached {
			c.scopeNameSet(e.scope, e.displayName)
		}
		scopes := make([]string, len(cached))
		for i, e := range cached {
			scopes[i] = e.scope
		}
		return scopes, nil
	}

	type result struct {
		entries []scopeEntry
		err     error
	}

	subCh := make(chan result, 1)
	mgCh := make(chan result, 1)

	go func() {
		entries, err := c.listSubscriptionEntries(ctx)
		subCh <- result{entries, err}
	}()

	go func() {
		entries, err := c.listManagementGroupEntries(ctx)
		mgCh <- result{entries, err}
	}()

	subResult := <-subCh
	mgResult := <-mgCh

	if subResult.err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", subResult.err)
	}

	// Populate the name cache from what we already fetched.
	all := subResult.entries
	if mgResult.err == nil {
		all = append(all, mgResult.entries...)
	}
	for _, e := range all {
		c.scopeNameSet(e.scope, e.displayName)
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no accessible subscriptions or management groups found — ensure you are logged in (az login)")
	}

	saveScopeCache(all)

	scopes := make([]string, len(all))
	for i, e := range all {
		scopes[i] = e.scope
	}
	return scopes, nil
}

func (c *Clients) listSubscriptionEntries(ctx context.Context) ([]scopeEntry, error) {
	client, err := armsubscriptions.NewClient(c.cred, &arm.ClientOptions{})
	if err != nil {
		return nil, err
	}
	var entries []scopeEntry
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, sub := range page.Value {
			if sub.SubscriptionID == nil {
				continue
			}
			scope := "/subscriptions/" + *sub.SubscriptionID
			name := *sub.SubscriptionID
			if sub.DisplayName != nil && *sub.DisplayName != "" {
				name = *sub.DisplayName
			}
			entries = append(entries, scopeEntry{scope: scope, displayName: name})
		}
	}
	return entries, nil
}

func (c *Clients) listManagementGroupEntries(ctx context.Context) ([]scopeEntry, error) {
	client, err := armmanagementgroups.NewClient(c.cred, &arm.ClientOptions{})
	if err != nil {
		return nil, err
	}
	var entries []scopeEntry
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, mg := range page.Value {
			if mg.Name == nil {
				continue
			}
			scope := "/providers/Microsoft.Management/managementGroups/" + *mg.Name
			name := *mg.Name
			if mg.Properties != nil && mg.Properties.DisplayName != nil && *mg.Properties.DisplayName != "" {
				name = *mg.Properties.DisplayName
			}
			entries = append(entries, scopeEntry{scope: scope, displayName: name})
		}
	}
	return entries, nil
}

// ResolveScopeName returns a human-readable display name for an ARM scope.
// It checks the in-memory cache first (populated during ListAccessibleScopes),
// then queries the API, then falls back to the last path segment.
func (c *Clients) ResolveScopeName(ctx context.Context, scope string) string {
	if name, ok := c.scopeNameGet(scope); ok {
		return name
	}

	name := c.fetchScopeName(ctx, scope)
	c.scopeNameSet(scope, name)
	return name
}

// fetchScopeName queries the Azure API to get a display name for a scope.
func (c *Clients) fetchScopeName(ctx context.Context, scope string) string {
	const mgPrefix = "/providers/Microsoft.Management/managementGroups/"

	if _, after, ok := strings.Cut(scope, mgPrefix); ok {
		mgID := after
		if slash := strings.Index(mgID, "/"); slash >= 0 {
			mgID = mgID[:slash]
		}
		mgClient, err := armmanagementgroups.NewClient(c.cred, &arm.ClientOptions{})
		if err == nil {
			resp, err := mgClient.Get(ctx, mgID, nil)
			if err == nil && resp.Properties != nil && resp.Properties.DisplayName != nil && *resp.Properties.DisplayName != "" {
				return *resp.Properties.DisplayName
			}
		}
	}

	// Only resolve subscription display name for a pure subscription scope
	// (/subscriptions/{id} with no further segments). Resource group and
	// resource scopes under a subscription should show their own name (last
	// path segment), not the subscription name.
	if after, ok := strings.CutPrefix(scope, "/subscriptions/"); ok {
		rest := after
		if !strings.Contains(rest, "/") {
			// Pure subscription scope — look up display name.
			subClient, err := armsubscriptions.NewClient(c.cred, &arm.ClientOptions{})
			if err == nil {
				resp, err := subClient.Get(ctx, rest, nil)
				if err == nil && resp.DisplayName != nil && *resp.DisplayName != "" {
					return *resp.DisplayName
				}
			}
		}
	}

	// Fallback: last meaningful path segment (resource name, RG name, etc.).
	return lastPathSegment(scope)
}

// lastPathSegment returns the final non-empty segment of an ARM scope path.
func lastPathSegment(scope string) string {
	s := strings.TrimRight(scope, "/")
	idx := strings.LastIndex(s, "/")
	if idx < 0 {
		return s
	}
	return s[idx+1:]
}
