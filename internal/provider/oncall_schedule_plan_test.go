package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// tfPlanOnCall runs PlanResourceChange through the real provider server, so the
// framework marks unconfigured computed attributes unknown on an edit exactly
// as it does under Terraform before ModifyPlan sees the plan.
func tfPlanOnCall(t *testing.T, prior, config, proposed onCallScheduleModel) onCallScheduleModel {
	t.Helper()
	ctx := context.Background()
	state, _ := schemaOf(t, NewOnCallScheduleResource())
	typ := state.Schema.Type().TerraformType(ctx)
	dv := func(m onCallScheduleModel) *tfprotov6.DynamicValue {
		s := state
		if d := s.Set(ctx, &m); d.HasError() {
			t.Fatalf("set: %v", d)
		}
		v, err := tfprotov6.NewDynamicValue(typ, s.Raw)
		if err != nil {
			t.Fatal(err)
		}
		return &v
	}
	srv := providerserver.NewProtocol6(New("test")())()
	if _, err := srv.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{}); err != nil {
		t.Fatal(err)
	}
	resp, err := srv.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName: "watchpoint_oncall_schedule", PriorState: dv(prior), Config: dv(config), ProposedNewState: dv(proposed),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("plan: %s: %s", d.Summary, d.Detail)
		}
	}
	raw, err := resp.PlannedState.Unmarshal(typ)
	if err != nil {
		t.Fatal(err)
	}
	var out onCallScheduleModel
	if d := (tfsdk.Plan{Schema: state.Schema, Raw: raw}).Get(ctx, &out); d.HasError() {
		t.Fatalf("plan get: %v", d)
	}
	return out
}

func onCallTestLayer(name string, prio int64, anchor string) onCallLayerModel {
	return onCallLayerModel{Name: types.StringValue(name), Priority: types.Int64Value(prio),
		RotationType: types.StringValue("Weekly"), RotationLengthDays: types.Int64Value(7),
		RotationLengthHours: types.Int64Null(), HandoffHour: types.Int64Value(0), AnchorDate: types.StringValue(anchor)}
}

func onCallUnsetLayer(name string) onCallLayerModel {
	return onCallLayerModel{Name: types.StringValue(name), Priority: types.Int64Null(), RotationType: types.StringNull(),
		RotationLengthDays: types.Int64Null(), RotationLengthHours: types.Int64Null(), HandoffHour: types.Int64Null(),
		AnchorDate: types.StringNull()}
}

// Editing anything on a schedule must not move an unconfigured layer anchor to
// today: the API recreates every layer on PATCH, so an omitted anchorDate
// restarts the rotation and changes who is on call. The prior date is matched
// by layer name, so a layer inserted above does not inherit another's anchor.
func TestOnCallScheduleEditKeepsLayerAnchors(t *testing.T) {
	f := newFakeAPI(t)
	item := tenantPath("/alerting", "/oncall-schedules") + "/ocs1"
	participant := func(order types.Int64) []onCallParticipantModel {
		return []onCallParticipantModel{{UserEmail: types.StringValue("ada@example.com"),
			UserID: types.StringNull(), UserName: types.StringNull(), OrderIndex: order}}
	}
	base := onCallScheduleModel{
		ID: types.StringValue("ocs1"), Name: types.StringValue("Primary"), Description: types.StringValue("old"),
		TimeZone: types.StringValue("UTC"), RotationType: types.StringValue("Weekly"), RotationLengthDays: types.Int64Value(7),
		RotationLengthHours: types.Int64Null(), HandoffHourUTC: types.Int64Value(0),
		AnchorUTC: types.StringValue("2026-01-01T00:00:00Z"), IsEnabled: types.BoolValue(true),
		Participants: participant(types.Int64Value(0)),
		CreatedAt:    types.StringValue("2026-01-01T00:00:00Z"), UpdatedAt: types.StringValue("2026-01-01T00:00:00Z"),
	}
	prior := base
	prior.Layers = []onCallLayerModel{onCallTestLayer("Weekend", 2, "2026-01-05"), onCallTestLayer("Nights", 1, "2026-02-01")}

	// Config: a new description and a new top layer; no layer sets an anchor.
	config := base
	config.ID, config.RotationLengthDays, config.AnchorUTC = types.StringNull(), types.Int64Null(), types.StringNull()
	config.CreatedAt, config.UpdatedAt = types.StringNull(), types.StringNull()
	config.Participants = participant(types.Int64Null())
	config.Description = types.StringValue("new")
	config.Layers = []onCallLayerModel{onCallUnsetLayer("Holidays"), onCallUnsetLayer("Weekend"), onCallUnsetLayer("Nights")}

	// Terraform core fills computed nulls from prior state by list index, which
	// would give Holidays the Weekend anchor if anything trusted position.
	proposed := config
	proposed.ID, proposed.RotationLengthDays, proposed.AnchorUTC = prior.ID, prior.RotationLengthDays, prior.AnchorUTC
	proposed.CreatedAt, proposed.UpdatedAt, proposed.Participants = prior.CreatedAt, prior.UpdatedAt, prior.Participants
	proposed.Layers = []onCallLayerModel{onCallTestLayer("Holidays", 2, "2026-01-05"),
		onCallTestLayer("Weekend", 1, "2026-02-01"), onCallUnsetLayer("Nights")}

	planned := tfPlanOnCall(t, prior, config, proposed)
	if !planned.Layers[0].AnchorDate.IsUnknown() {
		t.Fatalf("a new layer must stay unknown for the API to anchor, got %v", planned.Layers[0].AnchorDate)
	}
	if planned.Layers[1].AnchorDate.ValueString() != "2026-01-05" || planned.Layers[2].AnchorDate.ValueString() != "2026-02-01" {
		t.Fatalf("existing layers must keep their anchors by name, got %v / %v",
			planned.Layers[1].AnchorDate, planned.Layers[2].AnchorDate)
	}

	f.on("PATCH "+item, ok(map[string]any{"message": "Schedule updated."}))
	f.on("GET "+item, ok(map[string]any{
		"id": "ocs1", "name": "Primary", "description": "new", "timeZoneId": "UTC", "rotationType": "Weekly",
		"rotationLengthDays": 7, "handoffHourUtc": 0, "anchorUtc": "2026-01-01T00:00:00Z", "isEnabled": true,
		"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-10-04T00:00:00Z",
		"participants": []any{map[string]any{"id": "p1", "userEmail": "ada@example.com", "orderIndex": 0}},
		"layers": []any{
			map[string]any{"id": "l3", "name": "Holidays", "priority": 3, "rotationType": "Weekly", "rotationLengthDays": 7, "handoffHour": 0, "anchorDate": "2026-10-04T00:00:00Z"},
			map[string]any{"id": "l1", "name": "Weekend", "priority": 2, "rotationType": "Weekly", "rotationLengthDays": 7, "handoffHour": 0, "anchorDate": "2026-01-05T00:00:00Z"},
			map[string]any{"id": "l2", "name": "Nights", "priority": 1, "rotationType": "Weekly", "rotationLengthDays": 7, "handoffHour": 0, "anchorDate": "2026-02-01T00:00:00Z"},
		},
	}))
	tfUpdate(t, f, NewOnCallScheduleResource(), prior, planned)
	sent := f.last("PATCH " + item).Body["layers"].([]any)
	if _, has := sent[0].(map[string]any)["anchorDate"]; has {
		t.Fatalf("new layer anchor must be omitted, got %v", sent[0])
	}
	if sent[1].(map[string]any)["anchorDate"] != "2026-01-05" || sent[2].(map[string]any)["anchorDate"] != "2026-02-01" {
		t.Fatalf("PATCH must carry the prior anchors, got %v", sent)
	}
}

// The API returns layers by priority, highest first. Explicit priorities listed
// lowest first must still read back in the configured order, or the apply fails
// with "Provider produced inconsistent result after apply".
func TestOnCallScheduleLayersKeepConfiguredOrder(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/alerting", "/oncall-schedules")
	f.on("POST "+list, func(recordedRequest) (int, any) {
		return http.StatusCreated, map[string]any{"id": "ocs1", "name": "Primary"}
	})
	f.on("GET "+list+"/ocs1", ok(map[string]any{
		"id": "ocs1", "name": "Primary", "timeZoneId": "UTC", "rotationType": "Weekly", "rotationLengthDays": 7,
		"anchorUtc": "2026-10-01T00:00:00Z", "isEnabled": true,
		"createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-01T00:00:00Z",
		"participants": []any{map[string]any{"id": "p1", "userEmail": "ada@example.com", "orderIndex": 0}},
		"layers": []any{
			map[string]any{"id": "l2", "name": "High", "priority": 5, "rotationType": "Weekly", "rotationLengthDays": 7, "anchorDate": "2026-10-01T00:00:00Z"},
			map[string]any{"id": "l1", "name": "Low", "priority": 1, "rotationType": "Weekly", "rotationLengthDays": 7, "anchorDate": "2026-10-01T00:00:00Z"},
		},
	}))
	plan := onCallScheduleModel{
		Name:         types.StringValue("Primary"),
		Participants: []onCallParticipantModel{{UserEmail: types.StringValue("ada@example.com")}},
		Layers: []onCallLayerModel{
			{Name: types.StringValue("Low"), Priority: types.Int64Value(1)},
			{Name: types.StringValue("High"), Priority: types.Int64Value(5)},
		},
	}
	got := tfCreate(t, f, NewOnCallScheduleResource(), plan)
	if got.Layers[0].Name.ValueString() != "Low" || got.Layers[0].Priority.ValueInt64() != 1 ||
		got.Layers[1].Name.ValueString() != "High" || got.Layers[1].Priority.ValueInt64() != 5 {
		t.Fatalf("layers must follow config order, got %+v", got.Layers)
	}
}

// With no prior (an import) the API order is kept as is.
func TestAlignToKeepsAPIOrderWithoutPrior(t *testing.T) {
	got := alignTo([]string(nil), []string{"b", "a"}, func(p, g string) bool { return p == g })
	if got[0] != "b" || got[1] != "a" {
		t.Fatalf("got %v", got)
	}
}
