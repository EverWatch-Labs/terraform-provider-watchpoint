package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestStatusPageComponentGroupLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	groups := tenantPath("/status", "/pages/p1/groups")
	row := map[string]any{"id": "g1", "name": "Core", "description": nil, "position": 2, "collapsedByDefault": true}
	f.on("POST "+groups, func(recordedRequest) (int, any) { return http.StatusCreated, row })
	f.on("GET "+tenantPath("/status", "/pages/p1"), func(recordedRequest) (int, any) {
		return 200, map[string]any{"groups": []any{row}}
	})
	f.on("PATCH "+groups+"/g1", func(recordedRequest) (int, any) { return 200, row })
	f.on("DELETE "+groups+"/g1", func(recordedRequest) (int, any) { return http.StatusNoContent, nil })

	got := tfCreate(t, f, NewStatusPageComponentGroupResource(), statusPageComponentGroupModel{
		StatusPageID: types.StringValue("p1"), Name: types.StringValue("Core"),
		CollapsedByDefault: types.BoolValue(true), Position: types.Int64Unknown(),
	})
	if got.Position.ValueInt64() != 2 || !got.CollapsedByDefault.ValueBool() {
		t.Fatalf("state = %+v", got)
	}
	read, removed := tfRead(t, f, NewStatusPageComponentGroupResource(), got)
	if removed || read.Name.ValueString() != "Core" {
		t.Fatal("group must be found on the page detail")
	}
	tfUpdate(t, f, NewStatusPageComponentGroupResource(), got, got)
	if v, present := f.last("PATCH " + groups + "/g1").Body["description"]; !present || v != "" {
		t.Fatal("an absent description must be sent as \"\" so a removed one is cleared")
	}
	tfDelete(t, f, NewStatusPageComponentGroupResource(), got)

	row["id"] = "other"
	if _, removed := tfRead(t, f, NewStatusPageComponentGroupResource(), got); !removed {
		t.Fatal("a group no longer on the page must leave state")
	}
}

// Webhook subscribers return their secret once; chat subscribers go to the chat
// endpoint with their component filter, and the masked URL must not replace the
// configured one.
func TestStatusPageSubscriberChannels(t *testing.T) {
	f := newFakeAPI(t)
	base := tenantPath("/status", "/pages/p1/subscribers")
	f.on("POST "+base+"/webhooks", ok(map[string]any{
		"subscriber":    map[string]any{"id": "s1", "channel": "Webhook", "status": "Confirmed", "createdAt": "x", "componentFilterIds": []string{}},
		"signingSecret": "whsec",
	}))
	f.on("POST "+base+"/chat", ok(map[string]any{"id": "s2", "channel": "Slack", "status": "Confirmed",
		"createdAt": "x", "componentFilterIds": []string{"c1"}, "target": "hooks.slack.com/...abcd"}))
	f.on("GET "+base, ok([]any{map[string]any{"id": "s2", "channel": "Slack", "status": "Confirmed",
		"createdAt": "x", "componentFilterIds": []string{"c1"}}}))

	hook := tfCreate(t, f, NewStatusPageSubscriberResource(), statusPageSubscriberModel{
		StatusPageID: types.StringValue("p1"), Channel: types.StringValue("Webhook"),
		URL: types.StringValue("https://example.com/hook"), ComponentIDs: types.SetNull(types.StringType),
		SigningSecret: types.StringUnknown(),
	})
	if hook.SigningSecret.ValueString() != "whsec" || !hook.ComponentIDs.IsNull() {
		t.Fatalf("webhook state = %+v", hook)
	}

	comps, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"c1"})
	slackURL := "https://hooks.slack.com/services/T/B/secret"
	chat := tfCreate(t, f, NewStatusPageSubscriberResource(), statusPageSubscriberModel{
		StatusPageID: types.StringValue("p1"), Channel: types.StringValue("Slack"),
		URL: types.StringValue(slackURL), ComponentIDs: comps, SigningSecret: types.StringUnknown(),
	})
	body := f.last("POST " + base + "/chat").Body
	if body["channel"] != "Slack" || len(body["componentIds"].([]any)) != 1 {
		t.Fatalf("chat body = %v", body)
	}
	if !chat.SigningSecret.IsNull() {
		t.Fatal("a chat subscriber has no signing secret")
	}
	read, _ := tfRead(t, f, NewStatusPageSubscriberResource(), chat)
	if read.URL.ValueString() != slackURL {
		t.Fatal("the configured URL must survive the masked read")
	}
}

func TestIncidentUpdateTemplateLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/status", "/incident-update-templates")
	tpl := map[string]any{"id": "it1", "name": "Investigating", "status": "Investigating",
		"bodyMarkdown": "We are looking into {{component}}."}
	f.on("POST "+list, ok(tpl))
	f.on("GET "+list, ok(map[string]any{"variables": []string{"component"}, "templates": []any{tpl}}))
	f.on("PUT "+list+"/it1", ok(map[string]any{"id": "it1", "name": "Investigating", "status": nil,
		"bodyMarkdown": "We are looking into {{component}}."}))

	got := tfCreate(t, f, NewIncidentUpdateTemplateResource(), incidentUpdateTemplateModel{
		Name: types.StringValue("Investigating"), BodyMarkdown: types.StringValue("We are looking into {{component}}."),
		Status: types.StringValue("Investigating"),
	})
	if read, removed := tfRead(t, f, NewIncidentUpdateTemplateResource(), got); removed || read.Status.ValueString() != "Investigating" {
		t.Fatal("template must be found in the list")
	}
	next := got
	next.Status = types.StringNull()
	after := tfUpdate(t, f, NewIncidentUpdateTemplateResource(), got, next)
	if _, sent := f.last("PUT " + list + "/it1").Body["status"]; sent || !after.Status.IsNull() {
		t.Fatal("PUT is a full replace: an absent status is omitted and reads back null")
	}
}

// custom_css is stored sanitized and scoped, so the config text must be kept;
// removing it must send "" (always allowed, even below the plan tier).
func TestStatusPageCustomCSSKeepsConfig(t *testing.T) {
	var m statusPageModel
	m.CustomCSS = types.StringValue(".banner { color: red }")
	m.AllowedEmailDomains = types.SetNull(types.StringType)
	sanitized := ".wp-sp-root .banner{color:red}"
	var diags = &diagSink{d: new(diag.Diagnostics)}
	m.applyResponse(context.Background(), &statusPageResponse{CustomCSS: &sanitized}, diags)
	if m.CustomCSS.ValueString() != ".banner { color: red }" {
		t.Fatalf("custom_css = %q, want the configured text", m.CustomCSS.ValueString())
	}
	m.applyResponse(context.Background(), &statusPageResponse{}, diags)
	if !m.CustomCSS.IsNull() {
		t.Fatal("a page without CSS must read back null")
	}
	m.CustomCSS = types.StringNull()
	if b := m.toUpdateBody(context.Background(), diags); b.CustomCSS == nil || *b.CustomCSS != "" {
		t.Fatal("removing custom_css must send \"\"")
	}
}

func TestStatusPageComponentGroupIDClears(t *testing.T) {
	m := statusPageComponentModel{GroupID: types.StringNull()}
	if b := m.toUpdateBody(); b.GroupID == nil || *b.GroupID != "" {
		t.Fatal("a removed group_id must reach the API as \"\" (ungroup)")
	}
}
