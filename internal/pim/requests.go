package pim

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3"
)

// ScheduleRequestEntry holds display fields for one role assignment schedule request.
type ScheduleRequestEntry struct {
	RequestName    string // UUID assigned at creation; used for deduplication
	RoleName       string
	Scope          string // raw ARM scope
	ScopeDisplay   string // human-readable scope name
	ResourceType   string
	RequestType    string // "Activate", "Extend", "Deactivate", or raw ARM value
	Status         string // "Active", "Pending", "Denied", "Failed", "Canceled", or raw ARM value
	Justification  string
	RequestedAt    time.Time
	HasRequestedAt bool
	ExpiresAt      time.Time
	HasExpiry      bool
	RoleDefID      string
}

// IsPending reports whether the request is awaiting admin approval.
func (r *ScheduleRequestEntry) IsPending() bool {
	return r.Status == "Pending"
}

// ListRequests returns all role assignment schedule requests submitted by the
// current principal at the given ARM scope.
func (c *Clients) ListRequests(ctx context.Context, scope string) ([]ScheduleRequestEntry, error) {
	filter := "asRequestor()"
	opts := &armauthorization.RoleAssignmentScheduleRequestsClientListForScopeOptions{
		Filter: &filter,
	}

	pager := c.Requests.NewListForScopePager(scope, opts)

	var results []ScheduleRequestEntry
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list requests at scope %q: %w", scope, err)
		}
		for _, req := range page.Value {
			if req == nil || req.Properties == nil {
				continue
			}
			p := req.Properties

			reqName := ""
			if req.Name != nil {
				reqName = *req.Name
			}

			roleDefID := PtrString(p.RoleDefinitionID)
			roleName := c.ResolveRoleName(ctx, roleDefID)

			scopeStr := PtrString(p.Scope)
			if scopeStr == "" {
				scopeStr = scope
			}

			var requestedAt time.Time
			hasRequestedAt := false
			if p.CreatedOn != nil {
				requestedAt = *p.CreatedOn
				hasRequestedAt = true
			}

			var expiresAt time.Time
			hasExpiry := false
			if p.ScheduleInfo != nil && p.ScheduleInfo.Expiration != nil {
				if p.ScheduleInfo.Expiration.EndDateTime != nil {
					expiresAt = *p.ScheduleInfo.Expiration.EndDateTime
					hasExpiry = true
				}
			}

			results = append(results, ScheduleRequestEntry{
				RequestName:    reqName,
				RoleName:       roleName,
				Scope:          scopeStr,
				ScopeDisplay:   c.ResolveScopeName(ctx, scopeStr),
				ResourceType:   resourceTypeFromScope(scopeStr),
				RequestType:    humanizeRequestType(p.RequestType),
				Status:         humanizeRequestStatus(p.Status),
				Justification:  PtrString(p.Justification),
				RequestedAt:    requestedAt,
				HasRequestedAt: hasRequestedAt,
				ExpiresAt:      expiresAt,
				HasExpiry:      hasExpiry,
				RoleDefID:      roleDefID,
			})
		}
	}
	return results, nil
}

// ListRequestsForScopes queries multiple ARM scopes in parallel, combines the
// results, and deduplicates by request name. Per-scope errors are silently dropped.
func (c *Clients) ListRequestsForScopes(ctx context.Context, scopes []string) ([]ScheduleRequestEntry, error) {
	type result struct {
		entries []ScheduleRequestEntry
	}
	ch := make(chan result, len(scopes))
	for _, scope := range scopes {
		go func() {
			e, _ := c.ListRequests(ctx, scope) // ignore per-scope errors
			ch <- result{e}
		}()
	}
	var all []ScheduleRequestEntry
	for range scopes {
		r := <-ch
		all = append(all, r.entries...)
	}
	return deduplicateRequests(all), nil
}

// deduplicateRequests removes duplicate entries by request name (UUID).
// The same request can appear when querying both subscription and management
// group scopes if the request was submitted at a parent scope.
func deduplicateRequests(in []ScheduleRequestEntry) []ScheduleRequestEntry {
	seen := make(map[string]struct{}, len(in))
	out := make([]ScheduleRequestEntry, 0, len(in))
	for _, r := range in {
		key := r.RequestName
		if key == "" {
			// No name — use a composite key to avoid silently dropping entries.
			key = r.RoleDefID + "|" + normalizeScope(r.Scope) + "|" + r.RequestType
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	return out
}

func humanizeRequestType(rt *armauthorization.RequestType) string {
	if rt == nil {
		return ""
	}
	switch *rt {
	case armauthorization.RequestTypeSelfActivate:
		return "Activate"
	case armauthorization.RequestTypeSelfExtend:
		return "Extend"
	case armauthorization.RequestTypeSelfDeactivate:
		return "Deactivate"
	default:
		return string(*rt)
	}
}

func humanizeRequestStatus(s *armauthorization.Status) string {
	if s == nil {
		return ""
	}
	switch *s {
	case armauthorization.StatusProvisioned,
		armauthorization.StatusAdminApproved,
		armauthorization.StatusScheduleCreated:
		return "Active"
	case armauthorization.StatusPendingApproval,
		armauthorization.StatusPendingApprovalProvisioning,
		armauthorization.StatusPendingAdminDecision,
		armauthorization.StatusPendingEvaluation,
		armauthorization.StatusPendingProvisioning,
		armauthorization.StatusPendingScheduleCreation:
		return "Pending"
	case armauthorization.StatusAdminDenied,
		armauthorization.StatusDenied:
		return "Denied"
	case armauthorization.StatusFailed:
		return "Failed"
	case armauthorization.StatusCanceled:
		return "Canceled"
	case armauthorization.StatusRevoked:
		return "Revoked"
	default:
		return string(*s)
	}
}
