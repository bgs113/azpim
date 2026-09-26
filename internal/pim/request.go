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

// Mode controls what Activate, Deactivate and Extend do with the request they build.
type Mode int

const (
	Submit       Mode = iota // create the request (default)
	DryRun                   // build the request and send nothing
	ValidateOnly             // ask ARM to validate the request without creating it
)

// RequestOutcome is the status Azure reported for a newly submitted schedule
// request. Azure can accept a request without it taking effect yet (for
// example while it waits for approval), so callers must check Done before
// reporting success.
type RequestOutcome struct {
	Status string // raw API status, e.g. "Provisioned" or "PendingAdminDecision"; empty if not reported
	Check  string // "DryRun" or "Validated" when the request was not submitted
}

// State returns the status as `azpim requests` shows it ("Active", "Pending",
// "Revoked", ...), or the raw status when it has no friendlier name.
func (o RequestOutcome) State() string {
	if o.Check != "" {
		return o.Check
	}
	if o.Status == "" {
		return ""
	}
	s := armauthorization.Status(o.Status)
	return humanizeRequestStatus(&s)
}

// Done reports whether Azure says the change has already taken effect.
func (o RequestOutcome) Done() bool { return o.State() == "Active" || o.State() == "Revoked" }

// Pending reports whether the request is waiting on approval or provisioning.
func (o RequestOutcome) Pending() bool { return o.State() == "Pending" }

// outcomeFromStatus classifies a submitted request's status. Statuses that mean
// Azure rejected the request (denied, failed, timed out, canceled) are errors.
func outcomeFromStatus(s *armauthorization.Status) (RequestOutcome, error) {
	var o RequestOutcome
	if s != nil {
		o.Status = string(*s)
	}
	switch o.State() {
	case "Denied", "Failed", "Canceled":
		return o, fmt.Errorf("request was not accepted (Azure status: %s)", o.Status)
	}
	return o, nil
}

// submit creates a role assignment schedule request and returns the status
// Azure reported for it. Activate, Deactivate and Extend all go through here
// so none of them can report success without checking that status, and
// c.Mode can stop all of them from creating anything.
func (c *Clients) submit(ctx context.Context, scope string, req armauthorization.RoleAssignmentScheduleRequest) (RequestOutcome, error) {
	switch c.Mode {
	case DryRun:
		return RequestOutcome{Check: "DryRun"}, nil
	case ValidateOnly:
		resp, err := c.Requests.Validate(ctx, scope, uuid.New().String(), req, nil)
		if err != nil {
			return RequestOutcome{}, fmt.Errorf("validation failed: %w", err)
		}
		var status *armauthorization.Status
		if resp.Properties != nil {
			status = resp.Properties.Status
		}
		o, err := outcomeFromStatus(status)
		o.Check = "Validated"
		return o, err
	}
	resp, err := c.Requests.Create(ctx, scope, uuid.New().String(), req, nil)
	if err != nil {
		return RequestOutcome{}, err
	}
	var status *armauthorization.Status
	if resp.Properties != nil {
		status = resp.Properties.Status
	}
	return outcomeFromStatus(status)
}

// Activate creates a SelfActivate role assignment schedule request.
func (c *Clients) Activate(ctx context.Context, opts ActivateOptions) (RequestOutcome, error) {
	iso := durationToISO8601(opts.Duration)
	req := buildScheduleRequest(armauthorization.RequestTypeSelfActivate, opts.RoleDefID, opts.PrincipalID, &iso, opts.Justification, opts.TicketNumber, opts.TicketSystem)
	o, err := c.submit(ctx, opts.Scope, req)
	if err != nil {
		return o, fmt.Errorf("activate role %q at scope %q: %w", opts.RoleDefID, opts.Scope, err)
	}
	return o, nil
}

// DeactivateOptions controls a SelfDeactivate PIM request.
type DeactivateOptions struct {
	RoleDefID   string
	PrincipalID string
	Scope       string
}

// Deactivate creates a SelfDeactivate role assignment schedule request.
func (c *Clients) Deactivate(ctx context.Context, opts DeactivateOptions) (RequestOutcome, error) {
	req := buildScheduleRequest(armauthorization.RequestTypeSelfDeactivate, opts.RoleDefID, opts.PrincipalID, nil, "", "", "")
	o, err := c.submit(ctx, opts.Scope, req)
	if err != nil {
		return o, fmt.Errorf("deactivate role %q at scope %q: %w", opts.RoleDefID, opts.Scope, err)
	}
	return o, nil
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

// Extend creates a SelfExtend role assignment schedule request, asking for a
// new duration from the current time on an already-active assignment. Azure
// usually holds SelfExtend requests for admin approval, so check the outcome.
func (c *Clients) Extend(ctx context.Context, opts ExtendOptions) (RequestOutcome, error) {
	iso := durationToISO8601(opts.Duration)
	req := buildScheduleRequest(armauthorization.RequestTypeSelfExtend, opts.RoleDefID, opts.PrincipalID, &iso, opts.Justification, opts.TicketNumber, opts.TicketSystem)
	o, err := c.submit(ctx, opts.Scope, req)
	if err != nil {
		return o, fmt.Errorf("extend role %q at scope %q: %w", opts.RoleDefID, opts.Scope, err)
	}
	return o, nil
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
