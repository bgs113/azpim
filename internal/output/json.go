package output

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"azpim/internal/pim"
)

type eligibleJSON struct {
	RoleName       string `json:"role_name"`
	Scope          string `json:"scope"`
	ScopeDisplay   string `json:"scope_display"`
	ResourceType   string `json:"resource_type"`
	MembershipType string `json:"membership_type"`
	Condition      string `json:"condition,omitempty"`
	EndTime        string `json:"end_time,omitempty"`
	HasExpiry      bool   `json:"has_expiry"`
	RoleDefID      string `json:"role_definition_id"`
}

type activeJSON struct {
	RoleName             string `json:"role_name"`
	Resource             string `json:"resource"`
	ResourceType         string `json:"resource_type"`
	MembershipType       string `json:"membership_type"`
	Condition            string `json:"condition,omitempty"`
	State                string `json:"state"`
	EndTime              string `json:"end_time,omitempty"`
	TimeRemainingSeconds int64  `json:"time_remaining_seconds,omitempty"`
	TimeRemainingDisplay string `json:"time_remaining_display"`
	Scope                string `json:"scope"`
	RoleDefID            string `json:"role_definition_id"`
}

// PrintEligibleJSON writes eligible assignments as a JSON array to w.
func PrintEligibleJSON(w io.Writer, assignments []pim.EligibleAssignment) error {
	out := make([]eligibleJSON, 0, len(assignments))
	for _, a := range assignments {
		j := eligibleJSON{
			RoleName:       a.RoleName,
			Scope:          a.Scope,
			ScopeDisplay:   a.ScopeDisplay,
			ResourceType:   a.ResourceType,
			MembershipType: a.MembershipType,
			Condition:      a.Condition,
			HasExpiry:      a.HasExpiry,
			RoleDefID:      a.RoleDefID,
		}
		if a.HasExpiry && !a.EndTime.IsZero() {
			j.EndTime = a.EndTime.UTC().Format(time.RFC3339)
		}
		out = append(out, j)
	}
	return printJSON(w, out)
}

// PrintActiveJSON writes active assignments as a JSON array to w.
func PrintActiveJSON(w io.Writer, assignments []pim.ActiveAssignment, humanReadable bool) error {
	out := make([]activeJSON, 0, len(assignments))
	for _, a := range assignments {
		j := activeJSON{
			RoleName:             a.RoleName,
			Resource:             a.Resource,
			ResourceType:         a.ResourceType,
			MembershipType:       a.MembershipType,
			Condition:            a.Condition,
			State:                a.State,
			Scope:                a.Scope,
			RoleDefID:            a.RoleDefID,
			TimeRemainingDisplay: a.TimeRemaining(humanReadable),
		}
		if a.HasExpiry && !a.EndTime.IsZero() {
			j.EndTime = a.EndTime.UTC().Format(time.RFC3339)
			remaining := time.Until(a.EndTime)
			if remaining > 0 {
				j.TimeRemainingSeconds = int64(remaining.Seconds())
			}
		}
		out = append(out, j)
	}
	return printJSON(w, out)
}

type requestJSON struct {
	RequestName   string `json:"request_name"`
	RoleName      string `json:"role_name"`
	Scope         string `json:"scope"`
	ScopeDisplay  string `json:"scope_display"`
	ResourceType  string `json:"resource_type"`
	RequestType   string `json:"request_type"`
	Status        string `json:"status"`
	Justification string `json:"justification,omitempty"`
	RequestedAt   string `json:"requested_at,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	RoleDefID     string `json:"role_definition_id"`
}

// PrintRequestsJSON writes schedule requests as a JSON array to w.
// If pendingOnly is true, only requests with status "Pending" are included.
func PrintRequestsJSON(w io.Writer, requests []pim.ScheduleRequestEntry, pendingOnly bool) error {
	out := make([]requestJSON, 0, len(requests))
	for _, r := range requests {
		if pendingOnly && !r.IsPending() {
			continue
		}
		j := requestJSON{
			RequestName:   r.RequestName,
			RoleName:      r.RoleName,
			Scope:         r.Scope,
			ScopeDisplay:  r.ScopeDisplay,
			ResourceType:  r.ResourceType,
			RequestType:   r.RequestType,
			Status:        r.Status,
			Justification: r.Justification,
			RoleDefID:     r.RoleDefID,
		}
		if r.HasRequestedAt && !r.RequestedAt.IsZero() {
			j.RequestedAt = r.RequestedAt.UTC().Format(time.RFC3339)
		}
		if r.HasExpiry && !r.ExpiresAt.IsZero() {
			j.ExpiresAt = r.ExpiresAt.UTC().Format(time.RFC3339)
		}
		out = append(out, j)
	}
	return printJSON(w, out)
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}
