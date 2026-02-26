package output

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"azpim/internal/pim"
)

func TestPrintEligibleJSON(t *testing.T) {
	assignments := []pim.EligibleAssignment{
		{
			RoleName:       "Contributor",
			Scope:          "/subscriptions/sub1",
			ScopeDisplay:   "My Sub",
			ResourceType:   "Subscription",
			MembershipType: "Direct",
			HasExpiry:      false,
			RoleDefID:      "/providers/Microsoft.Authorization/roleDefinitions/abc",
		},
	}
	var buf bytes.Buffer
	if err := PrintEligibleJSON(&buf, assignments); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d entries, want 1", len(out))
	}
	if out[0]["role_name"] != "Contributor" {
		t.Errorf("role_name = %v, want Contributor", out[0]["role_name"])
	}
	if out[0]["scope"] != "/subscriptions/sub1" {
		t.Errorf("scope = %v, want /subscriptions/sub1", out[0]["scope"])
	}
	if out[0]["has_expiry"] != false {
		t.Errorf("has_expiry = %v, want false", out[0]["has_expiry"])
	}
}

func TestPrintEligibleJSONWithExpiry(t *testing.T) {
	end := time.Date(2025, 12, 31, 23, 59, 0, 0, time.UTC)
	assignments := []pim.EligibleAssignment{
		{RoleName: "Owner", HasExpiry: true, EndTime: end},
	}
	var buf bytes.Buffer
	if err := PrintEligibleJSON(&buf, assignments); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out[0]["has_expiry"] != true {
		t.Errorf("has_expiry = %v, want true", out[0]["has_expiry"])
	}
	if _, ok := out[0]["end_time"]; !ok {
		t.Error("expected end_time field to be present")
	}
}

func TestPrintRequestsJSON(t *testing.T) {
	requestedAt := time.Date(2025, 6, 15, 9, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2025, 6, 15, 13, 0, 0, 0, time.UTC)
	requests := []pim.ScheduleRequestEntry{
		{
			RequestName:    "uuid-abc",
			RoleName:       "Contributor",
			Scope:          "/subscriptions/sub1",
			ScopeDisplay:   "My Sub",
			ResourceType:   "Subscription",
			RequestType:    "Activate",
			Status:         "Active",
			Justification:  "incident response",
			RequestedAt:    requestedAt,
			HasRequestedAt: true,
			ExpiresAt:      expiresAt,
			HasExpiry:      true,
			RoleDefID:      "/providers/Microsoft.Authorization/roleDefinitions/abc",
		},
	}
	var buf bytes.Buffer
	if err := PrintRequestsJSON(&buf, requests, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d entries, want 1", len(out))
	}
	if out[0]["role_name"] != "Contributor" {
		t.Errorf("role_name = %v, want Contributor", out[0]["role_name"])
	}
	if out[0]["status"] != "Active" {
		t.Errorf("status = %v, want Active", out[0]["status"])
	}
	if out[0]["request_name"] != "uuid-abc" {
		t.Errorf("request_name = %v, want uuid-abc", out[0]["request_name"])
	}
	if _, ok := out[0]["requested_at"]; !ok {
		t.Error("expected requested_at field")
	}
	if _, ok := out[0]["expires_at"]; !ok {
		t.Error("expected expires_at field")
	}
}

func TestPrintRequestsJSONPendingFilter(t *testing.T) {
	requests := []pim.ScheduleRequestEntry{
		{RequestName: "a", RoleName: "Owner", Status: "Pending", RequestType: "Activate"},
		{RequestName: "b", RoleName: "Contributor", Status: "Active", RequestType: "Activate"},
	}
	var buf bytes.Buffer
	if err := PrintRequestsJSON(&buf, requests, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d entries, want 1 (only pending)", len(out))
	}
	if out[0]["role_name"] != "Owner" {
		t.Errorf("role_name = %v, want Owner", out[0]["role_name"])
	}
}

func TestPrintActiveJSON(t *testing.T) {
	end := time.Now().Add(4 * time.Hour)
	assignments := []pim.ActiveAssignment{
		{
			RoleName:       "Contributor",
			Resource:       "My Sub",
			ResourceType:   "Subscription",
			MembershipType: "Direct",
			State:          "Active",
			HasExpiry:      true,
			EndTime:        end,
			Scope:          "/subscriptions/sub1",
			RoleDefID:      "/providers/Microsoft.Authorization/roleDefinitions/abc",
		},
	}
	var buf bytes.Buffer
	if err := PrintActiveJSON(&buf, assignments, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d entries, want 1", len(out))
	}
	if out[0]["role_name"] != "Contributor" {
		t.Errorf("role_name = %v, want Contributor", out[0]["role_name"])
	}
	if out[0]["state"] != "Active" {
		t.Errorf("state = %v, want Active", out[0]["state"])
	}
	remaining, ok := out[0]["time_remaining_seconds"].(float64)
	if !ok || remaining <= 0 {
		t.Errorf("expected positive time_remaining_seconds, got %v", out[0]["time_remaining_seconds"])
	}
}
