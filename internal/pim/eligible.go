package pim

import (
	"context"
	"fmt"
	"path"
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
// at the given ARM scope. Pass "/" to list across the whole tenant in one query.
func (c *Clients) ListEligible(ctx context.Context, scope string) ([]EligibleAssignment, error) {
	filter := "asTarget()"
	opts := &armauthorization.RoleEligibilityScheduleInstancesClientListForScopeOptions{
		Filter: &filter,
	}

	pager := c.EligibleInstances.NewListForScopePager(scope, opts)

	var out []EligibleAssignment
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
			roleDefID := PtrString(p.RoleDefinitionID)
			roleName, scopeName := displayNames(p.ExpandedProperties, roleDefID, scopeStr)

			out = append(out, EligibleAssignment{
				RoleName:       roleName,
				Scope:          scopeStr,
				ScopeDisplay:   scopeName,
				ResourceType:   resourceTypeFromScope(scopeStr),
				MembershipType: membership,
				Condition:      PtrString(p.Condition),
				EndTime:        end,
				HasExpiry:      hasExpiry,
				RoleDefID:      roleDefID,
			})
		}
	}
	return deduplicateEligible(out), nil
}

// displayNames returns the role and scope display names from an API
// response's expandedProperties, falling back to the role definition GUID and
// the last scope path segment when a name is missing.
func displayNames(ep *armauthorization.ExpandedProperties, roleDefID, scope string) (roleName, scopeName string) {
	roleName, scopeName = roleDefGUID(roleDefID), path.Base(scope)
	if ep == nil {
		return roleName, scopeName
	}
	if ep.RoleDefinition != nil && PtrString(ep.RoleDefinition.DisplayName) != "" {
		roleName = *ep.RoleDefinition.DisplayName
	}
	if ep.Scope != nil && PtrString(ep.Scope.DisplayName) != "" {
		scopeName = *ep.Scope.DisplayName
	}
	return roleName, scopeName
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

// deduplicateEligible removes duplicate eligible assignments. When a group has
// an eligible assignment, Azure also creates a shadow "Direct" instance for
// every group member. The Azure Portal hides these shadows and shows only the
// Group entry. RoleDefID is normalised to its GUID because the same role can
// come back with a subscription- or tenant-prefixed ID.
func deduplicateEligible(in []EligibleAssignment) []EligibleAssignment {
	return dedupeGroupWins(in,
		func(a EligibleAssignment) string { return a.MembershipType },
		func(a EligibleAssignment) string { return roleDefGUID(a.RoleDefID) + "|" + normalizeScope(a.Scope) },
	)
}

// dedupeGroupWins removes duplicates keyed by key plus membership type, and
// drops shadow "Direct" entries whose key also has a "Group" entry (Azure
// creates a shadow Direct instance for every member of a group that has an
// eligible/active assignment; the Portal hides these and shows only the Group
// entry). Other membership types, such as "Permanent", are separate
// assignments and are kept. A two-pass approach ensures Group always wins
// regardless of the order the API returned results in.
func dedupeGroupWins[T any](in []T, membershipType func(T) string, key func(T) string) []T {
	// Pass 1: record which keys have a Group entry.
	hasGroup := make(map[string]struct{}, len(in))
	for _, a := range in {
		if membershipType(a) == "Group" {
			hasGroup[key(a)] = struct{}{}
		}
	}

	// Pass 2: drop shadow Direct entries where a Group entry exists, then
	// dedup the remainder so each key+membership appears at most once.
	seen := make(map[string]struct{}, len(in))
	out := make([]T, 0, len(in))
	for _, a := range in {
		k := key(a)
		if _, ok := hasGroup[k]; ok && membershipType(a) == "Direct" {
			continue // shadow Direct — a Group entry exists for this key
		}
		k += "|" + membershipType(a)
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
