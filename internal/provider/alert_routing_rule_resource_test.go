package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func strptr(s string) *string { return &s }

// The API stores the candidate list and the day list as CSV strings. Both are
// unordered sets in the schema, so the round trip has to survive the encoding.
func TestAlertRoutingRuleApplyResponse(t *testing.T) {
	var diags diag.Diagnostics
	start, end := int64(1320), int64(360)

	var m alertRoutingRuleModel
	m.applyResponse(context.Background(), &alertRoutingRuleResponse{
		ID: "arr_1", Name: "After hours to the on-call lead",
		Description: strptr("nights and weekends"),
		IsEnabled:   true, Priority: 20, StopProcessing: true,
		CreatedAt: "2026-09-01T00:00:00Z", UpdatedAt: "2026-09-02T00:00:00Z",
		Conditions: []alertRoutingConditionResponse{
			{ID: "c1", ConditionType: "MonitorTag", Operator: "In", Value: strptr("database,cache")},
			{ID: "c2", ConditionType: "TimeWindow", Operator: "In", TimeZone: strptr("America/New_York"),
				DaysOfWeek: strptr("1,2,3,4,5"), StartMinuteOfDay: &start, EndMinuteOfDay: &end},
		},
		Targets: []alertRoutingTargetResponse{
			{ID: "t1", TargetType: "AlertProfile", TargetID: "ap_1"},
		},
	}, &diagSink{d: &diags})

	if m.Priority.ValueInt64() != 20 || !m.StopProcessing.ValueBool() {
		t.Fatalf("priority/stop_processing = %d/%v, want 20/true", m.Priority.ValueInt64(), m.StopProcessing.ValueBool())
	}
	if len(m.Conditions) != 2 || len(m.Targets) != 1 {
		t.Fatalf("len(conditions)/len(targets) = %d/%d, want 2/1", len(m.Conditions), len(m.Targets))
	}
	if got := len(m.Conditions[0].Values.Elements()); got != 2 {
		t.Fatalf("len(values) = %d, want 2", got)
	}
	if got := len(m.Conditions[1].DaysOfWeek.Elements()); got != 5 {
		t.Fatalf("len(days_of_week) = %d, want 5", got)
	}
	if m.Conditions[1].EndMinuteOfDay.ValueInt64() != 360 {
		t.Fatalf("end_minute_of_day = %d, want 360", m.Conditions[1].EndMinuteOfDay.ValueInt64())
	}
	// A condition that is not a time window must not gain window fields.
	if !m.Conditions[0].TimeZone.IsNull() || !m.Conditions[0].StartMinuteOfDay.IsNull() {
		t.Fatal("absent time-window fields must map to null")
	}
}

// The API answers `[]` whether the caller sent no collection at all or an
// explicitly empty one, and Terraform treats null and [] as different values.
// Whichever the plan holds has to survive the response mapping unchanged, or
// the apply fails with "inconsistent result after apply".
func TestAlertRoutingRuleEmptyCollectionsKeepPlanShape(t *testing.T) {
	var diags diag.Diagnostics
	empty := &alertRoutingRuleResponse{ID: "arr_1", Name: "Catch-all", Conditions: nil, Targets: nil}

	var null alertRoutingRuleModel
	null.applyResponse(context.Background(), empty, &diagSink{d: &diags})
	if null.Conditions != nil || null.Targets != nil {
		t.Fatal("a plan that omitted the collections must stay null")
	}

	catchAll := alertRoutingRuleModel{
		Conditions: []alertRoutingConditionModel{},
		Targets:    []alertRoutingTargetModel{},
	}
	catchAll.applyResponse(context.Background(), empty, &diagSink{d: &diags})
	if catchAll.Conditions == nil || catchAll.Targets == nil {
		t.Fatal("a plan that set the collections to [] must stay []")
	}
}

// Nil conditions mean "leave them alone" on PATCH; an empty slice means "clear
// them". The body has to keep those apart, which is why neither field carries
// omitempty.
func TestAlertRoutingRuleBodyDistinguishesNilFromEmpty(t *testing.T) {
	var diags diag.Diagnostics
	ds := &diagSink{d: &diags}

	untouched := alertRoutingRuleModel{Name: types.StringValue("r")}
	if body := untouched.toBody(context.Background(), ds); body.Conditions != nil || body.Targets != nil {
		t.Fatal("omitted collections must marshal as null so the API leaves them alone")
	}

	cleared := alertRoutingRuleModel{
		Name:       types.StringValue("r"),
		Conditions: []alertRoutingConditionModel{},
		Targets:    []alertRoutingTargetModel{},
	}
	body := cleared.toBody(context.Background(), ds)
	if body.Conditions == nil || len(body.Conditions) != 0 || body.Targets == nil {
		t.Fatal("explicitly empty collections must marshal as [] so the API clears them")
	}
}
