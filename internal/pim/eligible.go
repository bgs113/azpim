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

	var results []EligibleAssignment
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

			roleDefID := PtrString(p.RoleDefinitionID)
			roleName := c.ResolveRoleName(ctx, roleDefID)

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

			results = append(results, EligibleAssignment{
				RoleName:       roleName,
				Scope:          scopeStr,
				ScopeDisplay:   c.ResolveScopeName(ctx, scopeStr),
				ResourceType:   resourceTypeFromScope(scopeStr),
				MembershipType: membership,
				Condition:      PtrString(p.Condition),
				EndTime:        end,
				HasExpiry:      hasExpiry,
				RoleDefID:      roleDefID,
			})
		}
	}
	return results, nil
}

// ListEligibleForScopes queries multiple ARM scopes in parallel, combines the
// results, and deduplicates by (RoleDefID, Scope, MembershipType). Per-scope
// errors (e.g. PIM not configured, 403) are dropped when at least one scope
// succeeds. If every scope fails the first error is returned.
func (c *Clients) ListEligibleForScopes(ctx context.Context, scopes []string) ([]EligibleAssignment, error) {
	type result struct {
		assignments []EligibleAssignment
		err         error
	}
	ch := make(chan result, len(scopes))
	for _, scope := range scopes {
		go func() {
			a, err := c.ListEligible(ctx, scope)
			ch <- result{a, err}
		}()
	}
	var all []EligibleAssignment
	var firstErr error
	for range scopes {
		r := <-ch
		all = append(all, r.assignments...)
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
	}
	if len(all) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return deduplicateEligible(all), nil
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
//
// A two-pass approach is used so that Group always wins regardless of the
// order goroutines returned their results (sort-then-dedup is fragile when
// Group and Direct entries come back in a non-deterministic order).
func deduplicateEligible(in []EligibleAssignment) []EligibleAssignment {
	// Pass 1: record which (roleGUID, scope) keys have a Group assignment.
	hasGroup := make(map[string]struct{}, len(in))
	for _, a := range in {
		if a.MembershipType == "Group" {
			hasGroup[roleDefGUID(a.RoleDefID)+"|"+normalizeScope(a.Scope)] = struct{}{}
		}
	}

	// Pass 2: drop shadow Direct entries where a Group entry exists, then
	// dedup the remainder so each (role, scope) appears at most once.
	seen := make(map[string]struct{}, len(in))
	out := make([]EligibleAssignment, 0, len(in))
	for _, a := range in {
		key := roleDefGUID(a.RoleDefID) + "|" + normalizeScope(a.Scope)
		if _, ok := hasGroup[key]; ok && a.MembershipType != "Group" {
			continue // shadow Direct — a Group entry exists for this role+scope
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
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
	case contains(scope, "/providers/Microsoft.Management/managementGroups"):
		return "Management group"
	case countSlash(scope) == 2: // /subscriptions/{id}
		return "Subscription"
	case contains(scope, "/resourceGroups/") && !contains(scope, "/providers/"):
		return "Resource group"
	case contains(scope, "/providers/"):
		return "Resource"
	default:
		return ""
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func countSlash(s string) int {
	n := 0
	for _, c := range s {
		if c == '/' {
			n++
		}
	}
	return n
}
