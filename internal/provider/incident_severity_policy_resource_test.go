package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The API has no single-severity GET: it returns the whole four-rung ladder and
// the provider picks its rung out. Enum.TryParse on the write side is
// case-insensitive, so an imported "sev1" must still find the "Sev1" row rather
// than reading as a deleted resource and silently dropping out of state.
func TestFindIncidentSeverityPolicyMatchesCaseInsensitively(t *testing.T) {
	rows := []incidentSeverityPolicyRow{
		{Severity: "Sev1", ResponseTargetMinutes: 5},
		{Severity: "Sev2", ResponseTargetMinutes: 15},
		{Severity: "Sev3", ResponseTargetMinutes: 60},
		{Severity: "Sev4", ResponseTargetMinutes: 240},
	}

	if got := findIncidentSeverityPolicy(rows, "sev2"); got == nil || got.ResponseTargetMinutes != 15 {
		t.Fatalf("expected the Sev2 row, got %v", got)
	}
	if got := findIncidentSeverityPolicy(rows, "Sev5"); got != nil {
		t.Fatalf("expected no row off the end of the ladder, got %v", got)
	}
}

// applyRow must not write the server's spelling of severity back into state: a
// Required attribute whose applied value differs from config fails the apply
// with "was cty.StringVal(\"sev1\"), but now cty.StringVal(\"Sev1\")". The
// effective values themselves DO come from the server, because they are the
// built-in default overlaid with the stored override.
func TestIncidentSeverityPolicyApplyRowKeepsConfiguredSeverity(t *testing.T) {
	m := incidentSeverityPolicyModel{Severity: types.StringValue("sev1")}
	escalation := "esc_1"
	m.applyRow(&incidentSeverityPolicyRow{
		Severity:               "Sev1",
		ResponseTargetMinutes:  5,
		RequirePostmortem:      true,
		PostmortemDueHours:     72,
		PageEscalationPolicyID: &escalation,
	})

	if m.Severity.ValueString() != "sev1" {
		t.Fatalf("severity was rewritten to %q", m.Severity.ValueString())
	}
	if m.ID.ValueString() != "sev1" {
		t.Fatalf("id must be the severity, got %q", m.ID.ValueString())
	}
	if m.ResponseTargetMinutes.ValueInt64() != 5 || !m.RequirePostmortem.ValueBool() ||
		m.PostmortemDueHours.ValueInt64() != 72 {
		t.Fatalf("effective policy not taken from the row: %+v", m)
	}
	if m.PageEscalationPolicyID.ValueString() != "esc_1" {
		t.Fatalf("escalation policy not taken from the row: %q", m.PageEscalationPolicyID.ValueString())
	}
}

// page_escalation_policy_id is Optional and NOT Computed, so a null from the API
// has to stay null. Writing "" back as a known value is what made Optional
// attributes fail their apply with "was null, but now cty.StringVal(\"\")".
func TestIncidentSeverityPolicyNullEscalationStaysNull(t *testing.T) {
	m := incidentSeverityPolicyModel{Severity: types.StringValue("Sev4")}
	m.applyRow(&incidentSeverityPolicyRow{Severity: "Sev4", ResponseTargetMinutes: 240, PostmortemDueHours: 168})
	if !m.PageEscalationPolicyID.IsNull() {
		t.Fatalf("expected null, got %q", m.PageEscalationPolicyID.ValueString())
	}
}

// An omitted Optional+Computed attribute must be absent from the PUT body: the
// controller treats an omitted field as "keep whatever was effective", so
// sending a zero there would clamp the rung to 1 minute behind the user's back.
func TestIncidentSeverityPolicyBodyOmitsUnsetFields(t *testing.T) {
	m := incidentSeverityPolicyModel{
		Severity:          types.StringValue("Sev3"),
		RequirePostmortem: types.BoolValue(true),
	}
	body := m.toBody()

	if body.ResponseTargetMinutes != nil || body.PostmortemDueHours != nil || body.PageEscalationPolicyID != nil {
		t.Fatalf("unset attributes leaked into the body: %+v", body)
	}
	if body.RequirePostmortem == nil || !*body.RequirePostmortem {
		t.Fatalf("a set attribute was dropped from the body: %+v", body)
	}
}
