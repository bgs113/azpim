package pim

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

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
		if strings.ToLower(e.DisplayName) == lower {
			return strings.TrimPrefix(e.Scope, "/subscriptions/"), nil
		}
	}
	return "", fmt.Errorf("no subscription found with name %q", nameOrID)
}

// scopeEntry pairs an ARM scope string with its human-readable display name.
type scopeEntry struct {
	Scope       string `json:"scope"`
	DisplayName string `json:"display_name"`
}

func (c *Clients) listSubscriptionEntries(ctx context.Context) ([]scopeEntry, error) {
	var entries []scopeEntry
	pager := c.Subscriptions.NewListPager(nil)
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
			entries = append(entries, scopeEntry{Scope: scope, DisplayName: name})
		}
	}
	return entries, nil
}

// ResolveManagementGroup returns the scope of the management group whose
// display name is name (case-insensitive). It only looks at management groups
// where the caller has an eligible or active assignment, which the tenant-wide
// assignment queries already name, because listing management groups needs a
// permission many users lack.
func (c *Clients) ResolveManagementGroup(ctx context.Context, name string) (string, error) {
	eligible, err := c.ListEligible(ctx, "/")
	if err != nil {
		return "", err
	}
	active, err := c.ListActive(ctx, "/", true)
	if err != nil {
		return "", err
	}
	var groups []scopeEntry
	for _, e := range eligible {
		groups = append(groups, scopeEntry{Scope: e.Scope, DisplayName: e.ScopeDisplay})
	}
	for _, a := range active {
		groups = append(groups, scopeEntry{Scope: a.Scope, DisplayName: a.Resource})
	}
	return matchManagementGroup(groups, name)
}

// matchManagementGroup picks the one management group scope among entries
// whose display name matches name, ignoring case and non-management-group scopes.
func matchManagementGroup(entries []scopeEntry, name string) (string, error) {
	seen := map[string]bool{}
	var all, matches []string
	for _, e := range entries {
		if resourceTypeFromScope(e.Scope) != "Management group" || seen[normalizeScope(e.Scope)] {
			continue
		}
		seen[normalizeScope(e.Scope)] = true
		label := fmt.Sprintf("  %s (ID %s)", e.DisplayName, path.Base(e.Scope))
		all = append(all, label)
		if strings.EqualFold(e.DisplayName, name) {
			matches = append(matches, e.Scope)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		msg := fmt.Sprintf("no management group with ID or name %q among those you have eligible or active assignments at; pass its ID with -m <id>", name)
		if len(all) > 0 {
			sort.Strings(all)
			msg += ". Management groups you have assignments at:\n" + strings.Join(all, "\n")
		}
		return "", fmt.Errorf("%s", msg)
	}
	var labels []string
	for _, m := range matches {
		labels = append(labels, fmt.Sprintf("  %s (ID %s)", name, path.Base(m)))
	}
	sort.Strings(labels)
	return "", fmt.Errorf("%d management groups are named %q; pass the ID with -m <id>:\n%s", len(matches), name, strings.Join(labels, "\n"))
}
