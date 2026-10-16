package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func workflowServerBody(conditionJSON string, actions []map[string]any) map[string]any {
	return map[string]any{
		"id": "wf1", "name": "Page on sev1", "description": nil, "isEnabled": true,
		"trigger": "alert.fired", "conditionJson": conditionJSON, "cooldownMinutes": 10,
		"actions": actions, "createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-01T00:00:00Z",
	}
}

// The API re-serialises the condition and action configs. Equal JSON must keep
// the operator's spelling (no perpetual diff), and the action list must reach
// the API in order with its index.
func TestWorkflowLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/alerting", "/workflows")
	item := tenantPath("/alerting", "/workflows/wf1")
	f.on("POST "+list, func(recordedRequest) (int, any) {
		return http.StatusCreated, workflowServerBody(`{"severity":"sev1"}`, []map[string]any{
			{"actionType": "Slack", "configJson": `{"channel":"#ops"}`},
			{"actionType": "Webhook", "configJson": `{"url":"https://hooks.example.com/x"}`},
		})
	})
	f.on("PATCH "+item, ok(workflowServerBody("", nil)))
	f.on("DELETE "+item, func(recordedRequest) (int, any) { return http.StatusNoContent, nil })

	plan := workflowModel{
		Name:            types.StringValue("Page on sev1"),
		IsEnabled:       types.BoolValue(true),
		Trigger:         types.StringValue("alert.fired"),
		ConditionJSON:   types.StringValue("{ \"severity\" : \"sev1\" }"),
		CooldownMinutes: types.Int64Value(10),
		Actions: []workflowActionModel{
			{ActionType: types.StringValue("Slack"), ConfigJSON: types.StringValue(`{ "channel": "#ops" }`)},
			{ActionType: types.StringValue("Webhook"), ConfigJSON: types.StringValue(`{"url": "https://hooks.example.com/x"}`)},
		},
	}
	got := tfCreate(t, f, NewWorkflowResource(), plan)

	if got.ID.ValueString() != "wf1" {
		t.Fatalf("id = %q", got.ID.ValueString())
	}
	if got.ConditionJSON.ValueString() != plan.ConditionJSON.ValueString() {
		t.Fatalf("condition_json = %q, want the config spelling kept", got.ConditionJSON.ValueString())
	}
	if got.Actions[0].ConfigJSON.ValueString() != `{ "channel": "#ops" }` {
		t.Fatalf("actions[0].config_json = %q", got.Actions[0].ConfigJSON.ValueString())
	}
	sent := f.last("POST " + list).Body
	actions := sent["actions"].([]any)
	if len(actions) != 2 || actions[1].(map[string]any)["orderIndex"].(float64) != 1 ||
		actions[1].(map[string]any)["actionType"] != "Webhook" {
		t.Fatalf("actions sent = %v", actions)
	}

	// Removing the condition and every action must clear them server-side.
	cleared := got
	cleared.ConditionJSON = types.StringNull()
	cleared.Actions = nil
	after := tfUpdate(t, f, NewWorkflowResource(), got, cleared)
	patch := f.last("PATCH " + item).Body
	if patch["conditionJson"] != "" {
		t.Fatalf("conditionJson = %v, want \"\" (the API's clear signal)", patch["conditionJson"])
	}
	if a, isList := patch["actions"].([]any); !isList || len(a) != 0 {
		t.Fatalf("actions = %v, want an empty list so the API removes them", patch["actions"])
	}
	if !after.ConditionJSON.IsNull() || after.Actions != nil {
		t.Fatal("cleared values must read back as null")
	}

	tfDelete(t, f, NewWorkflowResource(), after)
	if f.count("DELETE "+item) != 1 {
		t.Fatal("delete was not sent")
	}
}

func TestWorkflowReadRemovesOn404(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET "+tenantPath("/alerting", "/workflows/gone"), func(recordedRequest) (int, any) {
		return http.StatusNotFound, nil
	})
	_, removed := tfRead(t, f, NewWorkflowResource(), workflowModel{ID: types.StringValue("gone")})
	if !removed {
		t.Fatal("a 404 must drop the workflow from state")
	}
}
