package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestEscalationPolicyApplyResponse verifies the API response maps cleanly onto
// state, including the nested steps and a nil-steps short-circuit.
func TestEscalationPolicyApplyResponse(t *testing.T) {
	desc := "primary"
	schedID := "ocs_1"
	resp := &escalationPolicyResponse{
		ID:                    "esp_1",
		Name:                  "Platform",
		Description:           &desc,
		IsEnabled:             true,
		RepeatLastStep:        true,
		RepeatIntervalMinutes: 15,
		MaxRepeats:            3,
		CreatedAt:             "2026-01-01T00:00:00Z",
		UpdatedAt:             "2026-01-02T00:00:00Z",
		Steps: []escalationStepResponse{
			{ID: "step_1", StepOrder: 1, DelayMinutes: 0, TargetType: "OnCallSchedule", OnCallScheduleID: &schedID},
		},
	}

	var m escalationPolicyModel
	m.applyResponse(resp)

	if m.ID.ValueString() != "esp_1" {
		t.Fatalf("ID = %q, want esp_1", m.ID.ValueString())
	}
	if m.Description.ValueString() != "primary" {
		t.Fatalf("Description = %q, want primary", m.Description.ValueString())
	}
	if !m.RepeatLastStep.ValueBool() {
		t.Fatalf("RepeatLastStep = false, want true")
	}
	if len(m.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(m.Steps))
	}
	if m.Steps[0].TargetType.ValueString() != "OnCallSchedule" {
		t.Fatalf("Steps[0].TargetType = %q, want OnCallSchedule", m.Steps[0].TargetType.ValueString())
	}
	if m.Steps[0].OnCallScheduleID.ValueString() != "ocs_1" {
		t.Fatalf("Steps[0].OnCallScheduleID = %q, want ocs_1", m.Steps[0].OnCallScheduleID.ValueString())
	}
	// Optional pointer fields absent in the response must land as null, not "".
	if !m.Steps[0].TargetUserEmail.IsNull() {
		t.Fatalf("Steps[0].TargetUserEmail = %v, want null", m.Steps[0].TargetUserEmail)
	}

	// nil steps must clear the slice rather than leave a stale value.
	resp.Steps = nil
	m.applyResponse(resp)
	if m.Steps != nil {
		t.Fatalf("Steps = %v, want nil", m.Steps)
	}
}

// TestEscalationPolicyToBody verifies the model serializes onto the wire DTO,
// preserving step target fields and omitting unset optionals.
func TestEscalationPolicyToBody(t *testing.T) {
	m := escalationPolicyModel{
		Name:      types.StringValue("Platform"),
		IsEnabled: types.BoolValue(true),
		Steps: []escalationStepModel{
			{
				TargetType:      types.StringValue("User"),
				DelayMinutes:    types.Int64Value(10),
				TargetUserEmail: types.StringValue("lead@example.com"),
			},
		},
	}

	body := m.toBody()
	if body.Name == nil || *body.Name != "Platform" {
		t.Fatalf("body.Name = %v, want Platform", body.Name)
	}
	if len(body.Steps) != 1 {
		t.Fatalf("len(body.Steps) = %d, want 1", len(body.Steps))
	}
	s := body.Steps[0]
	if s.TargetType != "User" {
		t.Fatalf("step.TargetType = %q, want User", s.TargetType)
	}
	if s.TargetUserEmail == nil || *s.TargetUserEmail != "lead@example.com" {
		t.Fatalf("step.TargetUserEmail = %v, want lead@example.com", s.TargetUserEmail)
	}
	// Null model field must serialize to a nil pointer (omitted from JSON).
	if s.OnCallScheduleID != nil {
		t.Fatalf("step.OnCallScheduleID = %v, want nil", s.OnCallScheduleID)
	}
}

// TestOnCallScheduleRoundTrip verifies the schedule response maps onto state and
// back to the wire DTO with participants preserved.
func TestOnCallScheduleRoundTrip(t *testing.T) {
	name := "Ada"
	resp := &onCallScheduleResponse{
		ID:                 "ocs_1",
		Name:               "Primary",
		RotationType:       "Weekly",
		RotationLengthDays: 7,
		HandoffHourUTC:     16,
		AnchorUTC:          "2026-01-01T00:00:00Z",
		IsEnabled:          true,
		Participants: []onCallParticipantResponse{
			{ID: "p_1", UserEmail: "ada@example.com", UserName: &name, OrderIndex: 0},
		},
	}

	var m onCallScheduleModel
	m.applyResponse(resp)
	if m.RotationType.ValueString() != "Weekly" {
		t.Fatalf("RotationType = %q, want Weekly", m.RotationType.ValueString())
	}
	if len(m.Participants) != 1 || m.Participants[0].UserEmail.ValueString() != "ada@example.com" {
		t.Fatalf("participants not mapped: %+v", m.Participants)
	}

	body := m.toBody(false)
	if body.RotationType == nil || *body.RotationType != "Weekly" {
		t.Fatalf("body.RotationType = %v, want Weekly", body.RotationType)
	}
	if len(body.Participants) != 1 || body.Participants[0].UserEmail == nil || *body.Participants[0].UserEmail != "ada@example.com" {
		t.Fatalf("body participants not mapped: %+v", body.Participants)
	}
}
