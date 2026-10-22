package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The change-source signing secret is returned once and must survive reads.
func TestChangeSourceKeepsSecret(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/monitoring", "/maintenance/change-sources")
	row := map[string]any{"id": "cts_1", "kind": "servicenow", "name": "SN", "secretHint": "abcd",
		"webhookUrl": "https://api.example/hook", "scopeType": "group", "scopeValue": "g1",
		"approvedStates": []any{"Scheduled"}, "cancelledStates": []any{"Canceled"}, "startField": "start_date",
		"endField": "end_date", "notifyOnStart": true, "notifyOnEnd": true, "enabled": true}
	created := withRow(map[string]any{"secret": "s3cret-abcd"}, row)
	f.on("POST "+list, func(recordedRequest) (int, any) { return http.StatusCreated, created })
	f.on("GET "+list, ok([]any{row}))

	got := tfCreate(t, f, NewChangeSourceResource(), changeSourceModel{
		Kind: types.StringValue("servicenow"), Name: types.StringValue("SN"), ScopeType: types.StringValue("group"),
		ScopeValue: types.StringValue("g1"), ApprovedStates: types.SetUnknown(types.StringType),
		CancelledStates: types.SetUnknown(types.StringType), StartField: types.StringUnknown(), EndField: types.StringUnknown(),
		NotifyOnStart: types.BoolValue(true), NotifyOnEnd: types.BoolValue(true), Enabled: types.BoolValue(true),
		WebhookURL: types.StringUnknown(), Secret: types.StringUnknown(),
	})
	if b := f.last("POST " + list).Body; b["kind"] != "servicenow" || b["approvedStates"] != nil {
		t.Fatalf("post = %v", b)
	}
	after, _ := tfRead(t, f, NewChangeSourceResource(), got)
	if after.Secret.ValueString() != "s3cret-abcd" || after.WebhookURL.ValueString() != "https://api.example/hook" {
		t.Fatalf("read = %+v", after)
	}
}
