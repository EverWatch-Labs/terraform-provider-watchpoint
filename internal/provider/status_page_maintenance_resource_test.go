package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The API returns a null `recurrence` for a one-off window, but the schema
// defaults recurrence_frequency to "None" and recurrence_interval to 1. Leaving
// them null when the plan said "None" fails the apply with "was
// cty.StringVal(\"None\"), but now null".
func TestStatusPageMaintenanceApplyResponseMapsNullRecurrenceToDefaults(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	ds := &diagSink{d: &diags}

	var m statusPageMaintenanceModel
	m.applyResponse(ctx, &statusPageMaintenanceResponse{
		ID: "mw_1", StatusPageID: "sp_1", Title: "Database upgrade",
		ScheduledStartAt: "2026-07-01T02:00:00Z", ScheduledEndAt: "2026-07-01T03:00:00Z",
		Status: "Scheduled", SuppressAlerts: true, ComponentIDs: []string{},
	}, ds)

	if m.RecurrenceFrequency.ValueString() != "None" {
		t.Fatalf("expected frequency None, got %v", m.RecurrenceFrequency)
	}
	if m.RecurrenceInterval.ValueInt64() != 1 {
		t.Fatalf("expected interval 1, got %v", m.RecurrenceInterval)
	}
	if !m.RecurrenceByDay.IsNull() || !m.RecurrenceUntil.IsNull() || !m.RecurrenceCount.IsNull() {
		t.Fatal("the rest of the rule should be null for a one-off window")
	}
	// An empty component array is a null Set, not an empty one, for the same
	// reason: the configuration that omits it must round-trip unchanged.
	if !m.ComponentIDs.IsNull() {
		t.Fatalf("an empty component array should be a null set, got %v", m.ComponentIDs)
	}
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
}

// A recurring window carries the rule in a nested object, and byDay arrives as
// the API's canonical CSV ("MO,WE,FR"). It is unordered membership, so it has to
// land in a Set: a List would compare element order against configuration and
// fail the apply when the API returns the days in weekday order.
func TestStatusPageMaintenanceApplyResponseReadsTheRecurrenceRule(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	ds := &diagSink{d: &diags}

	byDay := "MO,WE,FR"
	until := "2026-12-31T00:00:00Z"
	count := int64(12)
	var m statusPageMaintenanceModel
	m.applyResponse(ctx, &statusPageMaintenanceResponse{
		ID: "mw_1", Status: "Scheduled",
		Recurrence: &statusPageMaintenanceRecurrence{
			Frequency: "Weekly", Interval: 2, ByDay: &byDay, Until: &until, Count: &count,
		},
	}, ds)

	if m.RecurrenceFrequency.ValueString() != "Weekly" || m.RecurrenceInterval.ValueInt64() != 2 {
		t.Fatalf("rule not read: %v / %v", m.RecurrenceFrequency, m.RecurrenceInterval)
	}
	days := m.RecurrenceByDay.Elements()
	if len(days) != 3 {
		t.Fatalf("expected three days, got %v", days)
	}
	if m.RecurrenceCount.ValueInt64() != 12 || m.RecurrenceUntil.ValueString() != until {
		t.Fatalf("bounds not read: %v / %v", m.RecurrenceCount, m.RecurrenceUntil)
	}
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
}

// componentIds and the recurrence rule are sent on every write, never omitted:
// the API reads an omitted component list as "leave them alone" and only
// re-evaluates the recurrence rule when a frequency is present, so removing
// either from configuration would otherwise be a silent no-op.
func TestStatusPageMaintenanceBodyAlwaysSendsComponentsAndRule(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	ds := &diagSink{d: &diags}

	m := statusPageMaintenanceModel{
		Title:               types.StringValue("Database upgrade"),
		ComponentIDs:        types.SetNull(types.StringType),
		RecurrenceFrequency: types.StringValue("None"),
		RecurrenceInterval:  types.Int64Value(1),
		RecurrenceByDay:     types.SetNull(types.StringType),
	}
	body := m.toBody(ctx, ds)

	if body.ComponentIDs == nil {
		t.Fatal("componentIds must be an empty array, never null")
	}
	if body.RecurrenceFrequency == nil || *body.RecurrenceFrequency != "None" {
		t.Fatalf("recurrenceFrequency must always be sent, got %v", body.RecurrenceFrequency)
	}
	if body.RecurrenceByDay != nil {
		t.Fatalf("an empty byDay set should be omitted, got %v", *body.RecurrenceByDay)
	}
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
}
