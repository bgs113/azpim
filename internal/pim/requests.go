package pim

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
)

// ScheduleRequestEntry holds display fields for one role assignment schedule request.
type ScheduleRequestEntry struct {
	RequestName    string // UUID assigned at creation; used for deduplication
	RoleName       string
	Scope          string // raw ARM scope
	ScopeDisplay   string // human-readable scope name
	ResourceType   string
	RequestType    string // "Activate", "Extend", "Deactivate", or raw ARM value
	Status         string // "Active", "Scheduled", "Pending", "Denied", "Failed", "Canceled", or raw ARM value
	Justification  string
	RequestedAt    time.Time
	HasRequestedAt bool
	StartsAt       time.Time // zero if Azure did not report a start time
	ExpiresAt      time.Time
	HasExpiry      bool
	RoleDefID      string
}

// ListRequests returns all role assignment schedule requests that target the
// current principal at the given ARM scope. Pass "/" to list across the whole
// tenant in one query. asTarget() is used because asRequestor() is rejected
// with InsufficientPermissions at "/"; for self-activation the two match.
func (c *Clients) ListRequests(ctx context.Context, scope string) ([]ScheduleRequestEntry, error) {
	filter := "asTarget()"
	opts := &armauthorization.RoleAssignmentScheduleRequestsClientListForScopeOptions{
		Filter: &filter,
	}

	pager := c.Requests.NewListForScopePager(scope, opts)

	var out []ScheduleRequestEntry
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

			var startsAt time.Time
			if p.ScheduleInfo != nil && p.ScheduleInfo.StartDateTime != nil {
				startsAt = *p.ScheduleInfo.StartDateTime
			}

			var expiresAt time.Time
			hasExpiry := false
			if p.ScheduleInfo != nil && p.ScheduleInfo.Expiration != nil {
				if p.ScheduleInfo.Expiration.EndDateTime != nil {
					expiresAt = *p.ScheduleInfo.Expiration.EndDateTime
					hasExpiry = true
				}
			}

			roleDefID := PtrString(p.RoleDefinitionID)
			roleName, scopeName := displayNames(p.ExpandedProperties, roleDefID, scopeStr)

			out = append(out, ScheduleRequestEntry{
				RequestName:    reqName,
				RoleName:       roleName,
				Scope:          scopeStr,
				ScopeDisplay:   scopeName,
				ResourceType:   resourceTypeFromScope(scopeStr),
				RequestType:    humanizeRequestType(p.RequestType),
				Status:         requestState(p.Status, startsAt, time.Now()),
				Justification:  PtrString(p.Justification),
				RequestedAt:    requestedAt,
				HasRequestedAt: hasRequestedAt,
				StartsAt:       startsAt,
				ExpiresAt:      expiresAt,
				HasExpiry:      hasExpiry,
				RoleDefID:      roleDefID,
			})
		}
	}
	return out, nil
}

// requestState is humanizeRequestStatus, except that a request Azure has
// accepted but whose start time is still ahead shows as "Scheduled".
func requestState(s *armauthorization.Status, start, now time.Time) string {
	state := humanizeRequestStatus(s)
	if start.After(now) && (state == "Active" || (s != nil && *s == armauthorization.StatusPendingScheduleCreation)) {
		return "Scheduled"
	}
	return state
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
		armauthorization.StatusGranted,
		armauthorization.StatusScheduleCreated:
		return "Active"
	case armauthorization.StatusPendingApproval,
		armauthorization.StatusPendingApprovalProvisioning,
		armauthorization.StatusPendingAdminDecision,
		armauthorization.StatusPendingEvaluation,
		armauthorization.StatusPendingExternalProvisioning,
		armauthorization.StatusPendingProvisioning,
		armauthorization.StatusPendingRevocation,
		armauthorization.StatusPendingScheduleCreation,
		armauthorization.StatusProvisioningStarted:
		return "Pending"
	case armauthorization.StatusAdminDenied,
		armauthorization.StatusDenied:
		return "Denied"
	case armauthorization.StatusFailed,
		armauthorization.StatusFailedAsResourceIsLocked,
		armauthorization.StatusInvalid,
		armauthorization.StatusTimedOut:
		return "Failed"
	case armauthorization.StatusCanceled:
		return "Canceled"
	case armauthorization.StatusRevoked:
		return "Revoked"
	default:
		return string(*s)
	}
}
