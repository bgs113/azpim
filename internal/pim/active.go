package pim

import (
	"context"
	"fmt"
	"sync"
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

// ListActive returns active (time-bound) PIM role assignments for the current principal.
// If includePermanent is true, directly-assigned permanent roles are also included.
func (c *Clients) ListActive(ctx context.Context, scope string, includePermanent bool) ([]ActiveAssignment, error) {
	filter := "asTarget()"
	opts := &armauthorization.RoleAssignmentScheduleInstancesClientListForScopeOptions{
		Filter: &filter,
	}

	pager := c.ActiveInstances.NewListForScopePager(scope, opts)

	// Phase 1: page through the API collecting raw fields (no ARM name lookups).
	type raw struct {
		roleDefID      string
		scopeStr       string
		membership     string
		condition      string
		state          string
		assignmentType string
		end            time.Time
		hasExpiry      bool
	}
	var raws []raw
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

			raws = append(raws, raw{
				roleDefID:      PtrString(p.RoleDefinitionID),
				scopeStr:       scopeStr,
				membership:     membership,
				condition:      PtrString(p.Condition),
				state:          state,
				assignmentType: assignmentType,
				end:            end,
				hasExpiry:      hasExpiry,
			})
		}
	}

	// Phase 2: resolve role names and scope names concurrently.
	// singleflight in ResolveRoleName/ResolveScopeName coalesces duplicate lookups.
	results := make([]ActiveAssignment, len(raws))
	var wg sync.WaitGroup
	for i, r := range raws {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = ActiveAssignment{
				RoleName:       c.ResolveRoleName(ctx, r.roleDefID),
				Resource:       c.ResolveScopeName(ctx, r.scopeStr),
				ResourceType:   resourceTypeFromScope(r.scopeStr),
				MembershipType: r.membership,
				Condition:      r.condition,
				State:          r.state,
				EndTime:        r.end,
				HasExpiry:      r.hasExpiry,
				Scope:          r.scopeStr,
				RoleDefID:      r.roleDefID,
				AssignmentType: r.assignmentType,
			}
		}()
	}
	wg.Wait()
	return results, nil
}

// ListActiveForScopes queries multiple ARM scopes in parallel, combines the
// results, and deduplicates by (RoleDefID, Scope, AssignmentType). Per-scope
// errors (e.g. PIM not configured, 403) are dropped when at least one scope
// succeeds. If every scope fails the first error is returned.
func (c *Clients) ListActiveForScopes(ctx context.Context, scopes []string, includePermanent bool) ([]ActiveAssignment, error) {
	type result struct {
		assignments []ActiveAssignment
		err         error
	}
	ch := make(chan result, len(scopes))
	for _, scope := range scopes {
		go func() {
			a, err := c.ListActive(ctx, scope, includePermanent)
			ch <- result{a, err}
		}()
	}
	var all []ActiveAssignment
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
	return deduplicateActive(all), nil
}

// deduplicateActive removes duplicate active assignments using the same
// two-pass strategy as deduplicateEligible: Group entries win over shadow
// Direct entries regardless of the order goroutines returned results.
func deduplicateActive(in []ActiveAssignment) []ActiveAssignment {
	hasGroup := make(map[string]struct{}, len(in))
	for _, a := range in {
		if a.MembershipType == "Group" {
			hasGroup[roleDefGUID(a.RoleDefID)+"|"+normalizeScope(a.Scope)] = struct{}{}
		}
	}

	seen := make(map[string]struct{}, len(in))
	out := make([]ActiveAssignment, 0, len(in))
	for _, a := range in {
		key := roleDefGUID(a.RoleDefID) + "|" + normalizeScope(a.Scope)
		if _, ok := hasGroup[key]; ok && a.MembershipType != "Group" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, a)
	}
	return out
}
