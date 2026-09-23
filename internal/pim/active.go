package pim

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
)

// ActiveAssignment holds display fields for one active (or permanent) assignment.
type ActiveAssignment struct {
	RoleName       string
	Resource       string // human-readable scope name (display name or last segment)
	ResourceType   string
	MembershipType string // Direct / Group / Permanent
	Condition      string
	State          string // Active / Permanent / Pending / Failed
	EndTime        time.Time
	HasExpiry      bool
	Scope          string // raw ARM scope, used for deactivate API call
	RoleDefID      string
	AssignmentType string // Activated / Assigned
}

// TimeRemaining returns a formatted string showing how long until the assignment expires.
// Returns "Permanent" for assignments without an end time.
// humanReadable controls whether to use "1h 32m 5s" (true) or "HH:MM:SS" (false).
func (a *ActiveAssignment) TimeRemaining(humanReadable bool) string {
	if !a.HasExpiry {
		return "Permanent"
	}
	d := time.Until(a.EndTime)
	if d <= 0 {
		return "Expired"
	}
	if humanReadable {
		return formatHuman(d)
	}
	return formatHMS(d)
}

func formatHMS(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func formatHuman(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// ListActive returns active (time-bound) PIM role assignments for the current
// principal at the given ARM scope. Pass "/" to list across the whole tenant in
// one query. If includePermanent is true, directly-assigned permanent roles are
// also included.
func (c *Clients) ListActive(ctx context.Context, scope string, includePermanent bool) ([]ActiveAssignment, error) {
	filter := "asTarget()"
	opts := &armauthorization.RoleAssignmentScheduleInstancesClientListForScopeOptions{
		Filter: &filter,
	}

	pager := c.ActiveInstances.NewListForScopePager(scope, opts)

	var out []ActiveAssignment
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list active assignments at scope %q: %w", scope, err)
		}
		for _, inst := range page.Value {
			if inst == nil || inst.Properties == nil {
				continue
			}
			p := inst.Properties

			// AssignmentType: "Activated" = PIM-activated, "Assigned" = permanent direct
			assignmentType := ""
			if p.AssignmentType != nil {
				assignmentType = string(*p.AssignmentType)
			}

			isPermanent := assignmentType == "Assigned"
			if isPermanent && !includePermanent {
				continue
			}

			membership := "Direct"
			if p.MemberType != nil && *p.MemberType == armauthorization.MemberTypeGroup {
				membership = "Group"
			}
			if isPermanent {
				membership = "Permanent"
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

			state := "Active"
			if isPermanent {
				state = "Permanent"
			}
			if p.Status != nil {
				switch *p.Status {
				case armauthorization.StatusFailed:
					state = "Failed"
				case armauthorization.StatusPendingApproval,
					armauthorization.StatusPendingApprovalProvisioning,
					armauthorization.StatusPendingEvaluation,
					armauthorization.StatusPendingProvisioning,
					armauthorization.StatusPendingScheduleCreation:
					state = "Pending"
				}
			}

			roleDefID := PtrString(p.RoleDefinitionID)
			roleName, scopeName := displayNames(p.ExpandedProperties, roleDefID, scopeStr)

			out = append(out, ActiveAssignment{
				RoleName:       roleName,
				Resource:       scopeName,
				ResourceType:   resourceTypeFromScope(scopeStr),
				MembershipType: membership,
				Condition:      PtrString(p.Condition),
				State:          state,
				EndTime:        end,
				HasExpiry:      hasExpiry,
				Scope:          scopeStr,
				RoleDefID:      roleDefID,
				AssignmentType: assignmentType,
			})
		}
	}
	return deduplicateActive(out), nil
}

// deduplicateActive removes duplicate active assignments using the same
// two-pass strategy as deduplicateEligible: Group entries win over shadow
// Direct entries regardless of the order the API returned results in.
func deduplicateActive(in []ActiveAssignment) []ActiveAssignment {
	return dedupeGroupWins(in,
		func(a ActiveAssignment) string { return a.MembershipType },
		func(a ActiveAssignment) string { return roleDefGUID(a.RoleDefID) + "|" + normalizeScope(a.Scope) },
	)
}
