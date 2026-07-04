package pim

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
)

// EligibleAssignment holds the display fields for one eligible role assignment.
type EligibleAssignment struct {
	RoleName       string
	Scope          string // raw ARM scope, used for activate API call
	ScopeDisplay   string // human-readable scope name, used for table output
	ResourceType   string
	MembershipType string // Direct / Group
	Condition      string
	EndTime        time.Time
	HasExpiry      bool
	RoleDefID      string // raw ARM ID, used for activate lookup
}

// ListEligible returns all eligible PIM role assignments for the current principal
// at the given ARM scope.
func (c *Clients) ListEligible(ctx context.Context, scope string) ([]EligibleAssignment, error) {
	filter := "asTarget()"
	opts := &armauthorization.RoleEligibilityScheduleInstancesClientListForScopeOptions{
		Filter: &filter,
	}

	pager := c.EligibleInstances.NewListForScopePager(scope, opts)

	// Phase 1: page through the API collecting raw fields (no ARM name lookups).
	type raw struct {
		roleDefID  string
		scopeStr   string
		membership string
		condition  string
		end        time.Time
		hasExpiry  bool
	}
	var raws []raw
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list eligible assignments at scope %q: %w", scope, err)
		}
		for _, inst := range page.Value {
			if inst == nil || inst.Properties == nil {
				continue
			}
			p := inst.Properties

			membership := "Direct"
			if p.MemberType != nil && *p.MemberType == armauthorization.MemberTypeGroup {
				membership = "Group"
			}

			var end time.Time
			hasExpiry := false
			if p.EndDateTime != nil {
				end = *p.EndDateTime
				hasExpiry = true
			}

			scopeStr := PtrString(p.Scope)
			if scopeStr == "" {
				scopeStr = scope
			}

			raws = append(raws, raw{
				roleDefID:  PtrString(p.RoleDefinitionID),
				scopeStr:   scopeStr,
				membership: membership,
				condition:  PtrString(p.Condition),
				end:        end,
				hasExpiry:  hasExpiry,
			})
		}
	}

	// Phase 2: resolve role names and scope names concurrently.
	// singleflight in ResolveRoleName/ResolveScopeName coalesces duplicate lookups.
	results := resolveConcurrently(raws, func(r raw) EligibleAssignment {
		return EligibleAssignment{
			RoleName:       c.ResolveRoleName(ctx, r.roleDefID),
			Scope:          r.scopeStr,
			ScopeDisplay:   c.ResolveScopeName(ctx, r.scopeStr),
			ResourceType:   resourceTypeFromScope(r.scopeStr),
			MembershipType: r.membership,
			Condition:      r.condition,
			EndTime:        r.end,
			HasExpiry:      r.hasExpiry,
			RoleDefID:      r.roleDefID,
		}
	})
	return results, nil
}

// ListEligibleForScopes queries multiple ARM scopes in parallel, combines the
// results, and deduplicates by (RoleDefID, Scope, MembershipType). Per-scope
// errors (e.g. PIM not configured, 403) are dropped when at least one scope
// succeeds. If every scope fails the first error is returned.
func (c *Clients) ListEligibleForScopes(ctx context.Context, scopes []string) ([]EligibleAssignment, error) {
	all, err := fetchForScopes(scopes, func(scope string) ([]EligibleAssignment, error) {
		return c.ListEligible(ctx, scope)
	})
	if err != nil {
		return nil, err
	}
	return deduplicateEligible(all), nil
}

// fetchForScopes calls fetch for every scope in parallel and combines the
// results. Per-scope errors are dropped when at least one scope succeeds; if
// every scope fails, the first error is returned.
func fetchForScopes[T any](scopes []string, fetch func(scope string) ([]T, error)) ([]T, error) {
	type result struct {
		items []T
		err   error
	}
	ch := make(chan result, len(scopes))
	for _, scope := range scopes {
		go func() {
			items, err := fetch(scope)
			ch <- result{items, err}
		}()
	}
	var all []T
	var firstErr error
	for range scopes {
		r := <-ch
		all = append(all, r.items...)
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
	}
	if len(all) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return all, nil
}

// FilterEligibleActive removes eligible assignments that are already active
// (i.e. have a current time-bound PIM activation). Permanent role assignments
// do not block re-activation and are not considered.
func FilterEligibleActive(eligible []EligibleAssignment, active []ActiveAssignment) []EligibleAssignment {
	activated := make(map[string]struct{}, len(active))
	for _, a := range active {
		if a.MembershipType == "Permanent" {
			continue
		}
		activated[roleDefGUID(a.RoleDefID)+"|"+normalizeScope(a.Scope)] = struct{}{}
	}
	out := make([]EligibleAssignment, 0, len(eligible))
	for _, e := range eligible {
		key := roleDefGUID(e.RoleDefID) + "|" + normalizeScope(e.Scope)
		if _, ok := activated[key]; ok {
			continue
		}
		out = append(out, e)
	}
	return out
}

// deduplicateEligible removes duplicate eligible assignments. Two sources of
// duplication are handled:
//
//  1. The same MG-scoped assignment is returned from every subscription scope
//     query (normalise RoleDefID to GUID; deduplicate by role+scope).
//
//  2. When a group has an eligible assignment, Azure also creates a shadow
//     "Direct" instance for every group member. The Azure Portal hides these
//     shadows and shows only the Group entry.
func deduplicateEligible(in []EligibleAssignment) []EligibleAssignment {
	return dedupeGroupWins(in,
		func(a EligibleAssignment) string { return a.MembershipType },
		func(a EligibleAssignment) string { return roleDefGUID(a.RoleDefID) + "|" + normalizeScope(a.Scope) },
	)
}

// dedupeGroupWins removes duplicates keyed by key, preferring a "Group"
// membershipType entry over shadow "Direct" entries sharing the same key
// (Azure creates a shadow Direct instance for every member of a group that
// has an eligible/active assignment; the Portal hides these and shows only
// the Group entry). A two-pass approach ensures Group always wins regardless
// of the order concurrent fetches returned results in.
func dedupeGroupWins[T any](in []T, membershipType func(T) string, key func(T) string) []T {
	// Pass 1: record which keys have a Group entry.
	hasGroup := make(map[string]struct{}, len(in))
	for _, a := range in {
		if membershipType(a) == "Group" {
			hasGroup[key(a)] = struct{}{}
		}
	}

	// Pass 2: drop shadow Direct entries where a Group entry exists, then
	// dedup the remainder so each key appears at most once.
	seen := make(map[string]struct{}, len(in))
	out := make([]T, 0, len(in))
	for _, a := range in {
		k := key(a)
		if _, ok := hasGroup[k]; ok && membershipType(a) != "Group" {
			continue // shadow Direct — a Group entry exists for this key
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, a)
	}
	return out
}

// roleDefGUID extracts the bare GUID from a role definition ARM ID.
// e.g. /subscriptions/{sub}/providers/Microsoft.Authorization/roleDefinitions/{guid} → {guid}
func roleDefGUID(roleDefID string) string {
	if idx := strings.LastIndex(roleDefID, "/"); idx >= 0 {
		return roleDefID[idx+1:]
	}
	return roleDefID
}

// normalizeScope lowercases and strips trailing slashes from an ARM scope
// to ensure consistent comparison across API responses.
func normalizeScope(scope string) string {
	return strings.ToLower(strings.TrimRight(scope, "/"))
}

// resourceTypeFromScope derives a short resource type label from an ARM scope string.
func resourceTypeFromScope(scope string) string {
	switch {
	case strings.Contains(scope, "/providers/Microsoft.Management/managementGroups"):
		return "Management group"
	case strings.Count(scope, "/") == 2: // /subscriptions/{id}
		return "Subscription"
	case strings.Contains(scope, "/resourceGroups/") && !strings.Contains(scope, "/providers/"):
		return "Resource group"
	case strings.Contains(scope, "/providers/"):
		return "Resource"
	default:
		return ""
	}
}
