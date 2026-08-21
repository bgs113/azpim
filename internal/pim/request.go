package pim

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
)

// ActivateOptions controls a SelfActivate PIM request.
type ActivateOptions struct {
	RoleDefID     string        // full ARM role definition ID
	PrincipalID   string        // object ID of the requesting principal
	Scope         string        // ARM scope
	Duration      time.Duration // how long to activate for
	Justification string
	TicketNumber  string
	TicketSystem  string
}

// ActivationOutcome describes the result of a SelfActivate request.
type ActivationOutcome struct {
	// Pending is true when the role's policy requires admin approval and the
	// activation has not yet been granted.
	Pending bool
}

// Activate creates a SelfActivate role assignment schedule request.
func (c *Clients) Activate(ctx context.Context, opts ActivateOptions) (ActivationOutcome, error) {
	reqName := uuid.New().String()
	iso := durationToISO8601(opts.Duration)

	req := buildScheduleRequest(armauthorization.RequestTypeSelfActivate, opts.RoleDefID, opts.PrincipalID, &iso, opts.Justification, opts.TicketNumber, opts.TicketSystem)

	resp, err := c.Requests.Create(ctx, opts.Scope, reqName, req, nil)
	if err != nil {
		return ActivationOutcome{}, fmt.Errorf("activate role %q at scope %q: %w", opts.RoleDefID, opts.Scope, err)
	}

	var pending bool
	if resp.Properties != nil && resp.Properties.Status != nil {
		switch *resp.Properties.Status {
		case armauthorization.StatusPendingApproval,
			armauthorization.StatusPendingApprovalProvisioning,
			armauthorization.StatusPendingEvaluation,
			armauthorization.StatusPendingProvisioning,
			armauthorization.StatusPendingScheduleCreation:
			pending = true
		}
	}
	return ActivationOutcome{Pending: pending}, nil
}

// DeactivateOptions controls a SelfDeactivate PIM request.
type DeactivateOptions struct {
	RoleDefID   string
	PrincipalID string
	Scope       string
}

// Deactivate creates a SelfDeactivate role assignment schedule request.
func (c *Clients) Deactivate(ctx context.Context, opts DeactivateOptions) error {
	reqName := uuid.New().String()

	req := buildScheduleRequest(armauthorization.RequestTypeSelfDeactivate, opts.RoleDefID, opts.PrincipalID, nil, "", "", "")

	_, err := c.Requests.Create(ctx, opts.Scope, reqName, req, nil)
	if err != nil {
		return fmt.Errorf("deactivate role %q at scope %q: %w", opts.RoleDefID, opts.Scope, err)
	}
	return nil
}

// ExtendOptions controls a SelfExtend PIM request.
type ExtendOptions struct {
	RoleDefID     string // full ARM role definition ID
	PrincipalID   string // object ID of the requesting principal
	Scope         string // ARM scope
	Duration      time.Duration
	Justification string
	TicketNumber  string
	TicketSystem  string
}

// Extend creates a SelfExtend role assignment schedule request, setting a new
// duration from the current time on an already-active assignment.
func (c *Clients) Extend(ctx context.Context, opts ExtendOptions) error {
	reqName := uuid.New().String()
	iso := durationToISO8601(opts.Duration)

	req := buildScheduleRequest(armauthorization.RequestTypeSelfExtend, opts.RoleDefID, opts.PrincipalID, &iso, opts.Justification, opts.TicketNumber, opts.TicketSystem)

	_, err := c.Requests.Create(ctx, opts.Scope, reqName, req, nil)
	if err != nil {
		return fmt.Errorf("extend role %q at scope %q: %w", opts.RoleDefID, opts.Scope, err)
	}
	return nil
}

// durationToISO8601 converts a Go duration to an ISO 8601 duration string.
// Azure PIM does not support sub-minute granularity, so the duration is
// rounded to the nearest minute before encoding.
func durationToISO8601(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h == 0:
		return fmt.Sprintf("PT%dM", m)
	case m == 0:
		return fmt.Sprintf("PT%dH", h)
	default:
		return fmt.Sprintf("PT%dH%dM", h, m)
	}
}

// buildScheduleRequest constructs a RoleAssignmentScheduleRequest shared by
// Activate, Deactivate, and Extend. iso is nil for Deactivate, which has no
// schedule/justification. TicketInfo is set only when a ticket number or
// system is provided.
func buildScheduleRequest(reqType armauthorization.RequestType, roleDefID, principalID string, iso *string, justification, ticketNumber, ticketSystem string) armauthorization.RoleAssignmentScheduleRequest {
	req := armauthorization.RoleAssignmentScheduleRequest{
		Properties: &armauthorization.RoleAssignmentScheduleRequestProperties{
			RoleDefinitionID: &roleDefID,
			PrincipalID:      &principalID,
			RequestType:      &reqType,
		},
	}

	if iso != nil {
		t := armauthorization.TypeAfterDuration
		req.Properties.ScheduleInfo = &armauthorization.RoleAssignmentScheduleRequestPropertiesScheduleInfo{
			Expiration: &armauthorization.RoleAssignmentScheduleRequestPropertiesScheduleInfoExpiration{
				Duration: iso,
				Type:     &t,
			},
		}
		req.Properties.Justification = &justification
	}

	if ticketNumber != "" || ticketSystem != "" {
		req.Properties.TicketInfo = &armauthorization.RoleAssignmentScheduleRequestPropertiesTicketInfo{
			TicketNumber: &ticketNumber,
			TicketSystem: &ticketSystem,
		}
	}

	return req
}
