package pim

import (
	"context"
	"fmt"
	"regexp"
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
