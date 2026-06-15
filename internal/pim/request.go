package pim

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/google/uuid"
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
	reqType := armauthorization.RequestTypeSelfActivate
	justification := opts.Justification

	req := armauthorization.RoleAssignmentScheduleRequest{
		Properties: &armauthorization.RoleAssignmentScheduleRequestProperties{
			RoleDefinitionID: &opts.RoleDefID,
			PrincipalID:      &opts.PrincipalID,
			RequestType:      &reqType,
			ScheduleInfo: &armauthorization.RoleAssignmentScheduleRequestPropertiesScheduleInfo{
				Expiration: &armauthorization.RoleAssignmentScheduleRequestPropertiesScheduleInfoExpiration{
					Duration: &iso,
					Type:     ptrExpirationTypeAfterDuration(),
				},
			},
			Justification: &justification,
		},
	}

	if opts.TicketNumber != "" || opts.TicketSystem != "" {
		req.Properties.TicketInfo = &armauthorization.RoleAssignmentScheduleRequestPropertiesTicketInfo{
			TicketNumber: &opts.TicketNumber,
			TicketSystem: &opts.TicketSystem,
		}
	}

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
	reqType := armauthorization.RequestTypeSelfDeactivate

	req := armauthorization.RoleAssignmentScheduleRequest{
		Properties: &armauthorization.RoleAssignmentScheduleRequestProperties{
			RoleDefinitionID: &opts.RoleDefID,
			PrincipalID:      &opts.PrincipalID,
			RequestType:      &reqType,
		},
	}

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
	reqType := armauthorization.RequestTypeSelfExtend
	justification := opts.Justification

	req := armauthorization.RoleAssignmentScheduleRequest{
		Properties: &armauthorization.RoleAssignmentScheduleRequestProperties{
			RoleDefinitionID: &opts.RoleDefID,
			PrincipalID:      &opts.PrincipalID,
			RequestType:      &reqType,
			ScheduleInfo: &armauthorization.RoleAssignmentScheduleRequestPropertiesScheduleInfo{
				Expiration: &armauthorization.RoleAssignmentScheduleRequestPropertiesScheduleInfoExpiration{
					Duration: &iso,
					Type:     ptrExpirationTypeAfterDuration(),
				},
			},
			Justification: &justification,
		},
	}

	if opts.TicketNumber != "" || opts.TicketSystem != "" {
		req.Properties.TicketInfo = &armauthorization.RoleAssignmentScheduleRequestPropertiesTicketInfo{
			TicketNumber: &opts.TicketNumber,
			TicketSystem: &opts.TicketSystem,
		}
	}

	_, err := c.Requests.Create(ctx, opts.Scope, reqName, req, nil)
	if err != nil {
		return fmt.Errorf("extend role %q at scope %q: %w", opts.RoleDefID, opts.Scope, err)
	}
	return nil
}

// durationToISO8601 converts a Go duration to an ISO 8601 duration string.
func durationToISO8601(d time.Duration) string {
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

func ptrExpirationTypeAfterDuration() *armauthorization.Type {
	t := armauthorization.TypeAfterDuration
	return &t
}
