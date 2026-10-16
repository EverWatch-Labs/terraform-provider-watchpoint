package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A zoned schedule with a custom hour cadence, an overnight restriction and a
// layer goes out in the API's shape, and the layer's midnight DateTime anchor
// reads back as the YYYY-MM-DD the schema takes.
func TestOnCallScheduleLayersAndRestrictions(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/alerting", "/oncall-schedules")
	item := list + "/ocs1"
	hours := int64(12)
	row := map[string]any{
		"id": "ocs1", "name": "Primary", "timeZoneId": "America/New_York", "rotationType": "Custom",
		"rotationLengthDays": 7, "rotationLengthHours": hours, "handoffHourUtc": 9,
		"anchorUtc": "2026-10-01T00:00:00Z", "isEnabled": true,
		"createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-01T00:00:00Z",
		"restrictions": []any{map[string]any{"days": []int{1, 2}, "startMinute": 1320, "endMinute": 360}},
		"participants": []any{map[string]any{"id": "p1", "userId": "", "userEmail": "ada@example.com", "orderIndex": 0}},
		"layers": []any{map[string]any{
			"id": "l1", "name": "Weekend", "priority": 1, "rotationType": "Weekly", "rotationLengthDays": 7,
			"handoffHour": 0, "anchorDate": "2026-10-05T00:00:00Z",
			"restrictions": []any{map[string]any{"days": []int{0, 6}, "startMinute": 0, "endMinute": 1440}},
			"participants": []any{map[string]any{"id": "p2", "userId": "", "userEmail": "bob@example.com", "orderIndex": 0}},
		}},
	}
	f.on("POST "+list, func(recordedRequest) (int, any) {
		return http.StatusCreated, map[string]any{"id": "ocs1", "name": "Primary"}
	})
	f.on("GET "+item, func(recordedRequest) (int, any) { return http.StatusOK, row })
	f.on("PATCH "+item, ok(map[string]any{"message": "Schedule updated."}))

	plan := onCallScheduleModel{
		Name: types.StringValue("Primary"), TimeZone: types.StringValue("America/New_York"),
		RotationType: types.StringValue("Custom"), RotationLengthHours: types.Int64Value(12),
		HandoffHourUTC: types.Int64Value(9), IsEnabled: types.BoolValue(true),
		Restrictions: []onCallRestrictionModel{{Days: []int64{1, 2}, StartMinute: types.Int64Value(1320), EndMinute: types.Int64Value(360)}},
		Participants: []onCallParticipantModel{{UserEmail: types.StringValue("ada@example.com")}},
		Layers: []onCallLayerModel{{
			Name: types.StringValue("Weekend"), AnchorDate: types.StringValue("2026-10-05"),
			Restrictions: []onCallRestrictionModel{{Days: []int64{0, 6}, StartMinute: types.Int64Value(0), EndMinute: types.Int64Value(1440)}},
			Participants: []onCallParticipantModel{{UserEmail: types.StringValue("bob@example.com")}},
		}},
	}
	got := tfCreate(t, f, NewOnCallScheduleResource(), plan)

	body := f.last("POST " + list).Body
	if body["timeZoneId"] != "America/New_York" || body["rotationType"] != "Custom" || body["rotationLengthHours"] != float64(12) {
		t.Fatalf("schedule fields not sent: %v", body)
	}
	r := body["restrictions"].([]any)[0].(map[string]any)
	if r["startMinute"] != float64(1320) || r["endMinute"] != float64(360) || len(r["days"].([]any)) != 2 {
		t.Fatalf("restriction = %v", r)
	}
	l := body["layers"].([]any)[0].(map[string]any)
	if l["name"] != "Weekend" || l["anchorDate"] != "2026-10-05" || len(l["participants"].([]any)) != 1 {
		t.Fatalf("layer = %v", l)
	}

	if len(got.Layers) != 1 || got.Layers[0].AnchorDate.ValueString() != "2026-10-05" {
		t.Fatalf("layer anchor must read back as a date, got %+v", got.Layers)
	}
	if got.Layers[0].Priority.ValueInt64() != 1 || got.TimeZone.ValueString() != "America/New_York" {
		t.Fatalf("state = %+v", got)
	}
	// userId "" is the API's "no user"; it must not turn a null config into "".
	if !got.Participants[0].UserID.IsNull() {
		t.Fatal("an email-only participant must keep a null user_id")
	}

	// Removing layers, restrictions and custom hours from config must clear them:
	// [] and 0 are the API's clear signals, an omitted field would keep them.
	next := got
	next.Layers, next.Restrictions = nil, nil
	next.RotationType, next.RotationLengthHours = types.StringValue("Weekly"), types.Int64Null()
	row["layers"], row["restrictions"], row["rotationLengthHours"], row["rotationType"] = []any{}, []any{}, nil, "Weekly"
	after := tfUpdate(t, f, NewOnCallScheduleResource(), got, next)

	patch := f.last("PATCH " + item)
	if layers, present := patch.Body["layers"]; !present || len(layers.([]any)) != 0 {
		t.Fatalf("layers = %v (present %v), want []", layers, present)
	}
	if rs, present := patch.Body["restrictions"]; !present || len(rs.([]any)) != 0 {
		t.Fatalf("restrictions = %v, want []", rs)
	}
	if patch.Body["rotationLengthHours"] != float64(0) {
		t.Fatalf("rotationLengthHours = %v, want 0", patch.Body["rotationLengthHours"])
	}
	if after.Layers != nil || after.Restrictions != nil || !after.RotationLengthHours.IsNull() {
		t.Fatalf("cleared collections must read back null: %+v", after)
	}
}

// On create an unset collection is sent as null ("not given"), never as [].
func TestOnCallScheduleCreateOmitsUnsetCollections(t *testing.T) {
	m := onCallScheduleModel{Name: types.StringValue("x"), Participants: []onCallParticipantModel{}}
	b := m.toBody(false)
	if b.Layers != nil || b.Restrictions != nil || b.RotationLengthHours != nil {
		t.Fatalf("create body = %+v", b)
	}
}
