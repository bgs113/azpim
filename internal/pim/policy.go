package pim

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
)

// FetchMaxActivationDuration queries the role management policy to find the
// maximum allowed activation duration for the given role at the given scope.
// Results are cached in memory for the lifetime of the Clients instance.
// Returns (0, nil) when no maximum is configured for the role.
// Returns (0, err) on API or parse failures; callers should warn the user and
// proceed without a duration cap.
func (c *Clients) FetchMaxActivationDuration(ctx context.Context, scope, roleDefID string) (time.Duration, error) {
	roleGUID := roleDefGUID(roleDefID)
	cacheKey := roleGUID + "|" + scope

	c.mu.Lock()
	if d, ok := c.policyCache[cacheKey]; ok {
		c.mu.Unlock()
		return d, nil
	}
	c.mu.Unlock()

	result, err := c.fetchMaxActivationDuration(ctx, scope, roleGUID)
	if err == nil {
		c.mu.Lock()
		c.policyCache[cacheKey] = result
		c.mu.Unlock()
	}
	return result, err
}

func (c *Clients) fetchMaxActivationDuration(ctx context.Context, scope, roleGUID string) (time.Duration, error) {
	pager := c.PolicyAssignments.NewListForScopePager(scope, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("list policy assignments at scope %q: %w", scope, err)
		}
		for _, pa := range page.Value {
			if pa == nil || pa.Properties == nil {
				continue
			}
			if roleDefGUID(PtrString(pa.Properties.RoleDefinitionID)) != roleGUID {
				continue
			}
			policyID := PtrString(pa.Properties.PolicyID)
			if policyID == "" {
				continue
			}
			policyName := lastPathSegment(policyID)
			resp, err := c.Policies.Get(ctx, scope, policyName, nil)
			if err != nil {
				return 0, fmt.Errorf("get policy %q at scope %q: %w", policyName, scope, err)
			}
			if resp.Properties == nil {
				return 0, fmt.Errorf("policy %q returned nil properties", policyName)
			}
			for _, ruleI := range resp.Properties.EffectiveRules {
				if ruleI == nil {
					continue
				}
				rule, ok := ruleI.(*armauthorization.RoleManagementPolicyExpirationRule)
				if !ok {
					continue
				}
				// The activation expiration rule targets EndUser callers.
				// The eligibility expiration rule targets Admin callers.
				if rule.Target == nil || PtrString(rule.Target.Caller) != "EndUser" {
					continue
				}
				if rule.MaximumDuration == nil {
					return 0, nil // policy exists but no max duration configured
				}
				d, err := parseISO8601Duration(*rule.MaximumDuration)
				if err != nil {
					return 0, fmt.Errorf("parse maximum duration %q: %w", *rule.MaximumDuration, err)
				}
				return d, nil
			}
		}
	}
	return 0, nil // no matching policy assignment found — PIM not configured for this role
}

// parseISO8601Duration parses a subset of ISO 8601 duration strings as
// returned by Azure PIM policy rules (e.g. "PT8H", "PT30M", "P1D", "PT8H30M").
func parseISO8601Duration(s string) (time.Duration, error) {
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("invalid ISO 8601 duration %q: must start with P", s)
	}
	rest := s[1:]

	var total time.Duration
	inTime := false

	for len(rest) > 0 {
		if rest[0] == 'T' {
			inTime = true
			rest = rest[1:]
			continue
		}

		// Read numeric value.
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(rest) {
			return 0, fmt.Errorf("invalid ISO 8601 duration %q", s)
		}

		n, _ := strconv.Atoi(rest[:i])
		unit := rest[i]
		rest = rest[i+1:]

		switch unit {
		case 'D':
			total += time.Duration(n) * 24 * time.Hour
		case 'H':
			total += time.Duration(n) * time.Hour
		case 'M':
			if !inTime {
				return 0, fmt.Errorf("month designator in ISO 8601 duration not supported: %q", s)
			}
			total += time.Duration(n) * time.Minute
		case 'S':
			total += time.Duration(n) * time.Second
		default:
			return 0, fmt.Errorf("unknown unit %q in ISO 8601 duration %q", unit, s)
		}
	}

	return total, nil
}

// FormatDuration formats a time.Duration as a compact Go duration string
// suitable for use as a prompt default (e.g. "8h", "1h30m").
func FormatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}
