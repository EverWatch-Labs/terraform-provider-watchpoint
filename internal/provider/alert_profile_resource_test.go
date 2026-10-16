package provider

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The new chat channels and the GitHub tracker reach the API with their own
// fields; the write-only bot token, which GET never returns, survives in state
// and is sent again on every save (channels are recreated wholesale, so a
// missing token would be wiped server-side).
func TestAlertProfileChatAndTrackerChannels(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/alerting", "/alert-profiles")
	item := list + "/ap1"
	prio := int64(1)
	row := map[string]any{
		"id": "ap1", "name": "Ops", "isEnabled": true, "cooldownMinutes": 5, "consecutiveFailures": 1,
		"alertOnRecovery": true, "deduplicateAcrossRegions": true, "ownerGroupId": "g1",
		"createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-01T00:00:00Z",
		"channels": []any{
			map[string]any{"id": "c1", "type": "Pushover", "isEnabled": true, "botRecipient": "u-key",
				"pushoverPriority": prio, "pagerDutyAutoResolve": true, "opsGenieAutoResolve": true,
				"ticketAutoClose": true, "hasBotToken": true},
			map[string]any{"id": "c2", "type": "Ticketing", "isEnabled": true, "ticketProvider": "github",
				"ticketProjectKey": "acme/api", "ticketApiToken": "{{vault.GH}}",
				"pagerDutyAutoResolve": true, "opsGenieAutoResolve": true, "ticketAutoClose": true},
		},
	}
	f.on("POST "+list, func(recordedRequest) (int, any) {
		return http.StatusCreated, map[string]any{"id": "ap1", "name": "Ops"}
	})
	f.on("GET "+item, func(recordedRequest) (int, any) { return http.StatusOK, row })
	f.on("PATCH "+item, ok(map[string]any{"message": "ok"}))

	base := alertProfileChannelModel{IsEnabled: types.BoolValue(true), PagerDutyAutoResolve: types.BoolValue(true),
		OpsGenieAutoResolve: types.BoolValue(true), TicketAutoClose: types.BoolValue(true)}
	pushover := base
	pushover.Type, pushover.BotToken = types.StringValue("Pushover"), types.StringValue("app-token")
	pushover.BotRecipient, pushover.PushoverPriority = types.StringValue("u-key"), types.Int64Value(1)
	github := base
	github.Type, github.TicketProvider = types.StringValue("Ticketing"), types.StringValue("github")
	github.TicketProjectKey, github.TicketAPIToken = types.StringValue("acme/api"), types.StringValue("{{vault.GH}}")

	plan := alertProfileModel{
		Name: types.StringValue("Ops"), IsDefault: types.BoolValue(false), IsEnabled: types.BoolValue(true),
		CooldownMinutes: types.Int64Value(5), ConsecutiveFailures: types.Int64Value(1),
		AlertOnRecovery: types.BoolValue(true), DeduplicateAcrossRegions: types.BoolValue(true),
		OwnerGroupID: types.StringValue("g1"), Channels: []alertProfileChannelModel{pushover, github},
	}
	got := tfCreate(t, f, NewAlertProfileResource(), plan)

	body := f.last("POST " + list).Body
	if body["ownerGroupId"] != "g1" {
		t.Fatalf("ownerGroupId = %v", body["ownerGroupId"])
	}
	ch := body["channels"].([]any)
	p, g := ch[0].(map[string]any), ch[1].(map[string]any)
	if p["type"] != "Pushover" || p["botToken"] != "app-token" || p["pushoverPriority"] != float64(1) {
		t.Fatalf("pushover channel = %v", p)
	}
	if g["ticketProvider"] != "github" || g["ticketProjectKey"] != "acme/api" {
		t.Fatalf("github channel = %v", g)
	}
	if got.Channels[0].BotToken.ValueString() != "app-token" {
		t.Fatal("write-only bot_token must be kept from the plan, not nulled by the read")
	}
	if got.Channels[1].TicketProvider.ValueString() != "github" || got.OwnerGroupID.ValueString() != "g1" {
		t.Fatalf("state = %+v", got)
	}

	// A refresh keeps the token too, and an update re-sends it while clearing
	// the owner with "".
	read, _ := tfRead(t, f, NewAlertProfileResource(), got)
	if read.Channels[0].BotToken.ValueString() != "app-token" {
		t.Fatal("refresh dropped the write-only bot_token")
	}
	next := read
	next.OwnerGroupID = types.StringNull()
	row["ownerGroupId"] = nil
	after := tfUpdate(t, f, NewAlertProfileResource(), read, next)
	patch := f.last("PATCH " + item).Body
	if v, present := patch["ownerGroupId"]; !present || v != "" {
		t.Fatalf("ownerGroupId = %v, want \"\"", v)
	}
	if patch["channels"].([]any)[0].(map[string]any)["botToken"] != "app-token" {
		t.Fatal("update must re-send the bot token or the API clears it")
	}
	if !after.OwnerGroupID.IsNull() {
		t.Fatal("cleared owner must read back null")
	}
}

// The API reads channels back with no ORDER BY. A swapped response must still
// land in the configured order, with each write-only secret on its own channel.
func TestAlertProfileChannelsAlignedToConfig(t *testing.T) {
	f := newFakeAPI(t)
	item := tenantPath("/alerting", "/alert-profiles") + "/ap1"
	f.on("GET "+item, ok(map[string]any{
		"id": "ap1", "name": "Ops", "isEnabled": true, "cooldownMinutes": 5, "consecutiveFailures": 1,
		"createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-01T00:00:00Z",
		"channels": []any{
			map[string]any{"id": "c2", "type": "Telegram", "isEnabled": true, "botRecipient": "chat-b"},
			map[string]any{"id": "c1", "type": "Telegram", "isEnabled": true, "botRecipient": "chat-a"},
			map[string]any{"id": "c3", "type": "Slack", "isEnabled": true, "slackWebhookUrl": "https://hooks.slack.com/x"},
		},
	}))
	ch := func(typ, recipient, token string) alertProfileChannelModel {
		return alertProfileChannelModel{Type: types.StringValue(typ), IsEnabled: types.BoolValue(true),
			BotRecipient: types.StringValue(recipient), BotToken: types.StringValue(token),
			PagerDutyAutoResolve: types.BoolValue(false), OpsGenieAutoResolve: types.BoolValue(false),
			TicketAutoClose: types.BoolValue(false)}
	}
	slack := alertProfileChannelModel{Type: types.StringValue("Slack"), IsEnabled: types.BoolValue(true),
		SlackWebhookURL: types.StringValue("https://hooks.slack.com/x"), SlackSigningSecret: types.StringValue("sign"),
		PagerDutyAutoResolve: types.BoolValue(false), OpsGenieAutoResolve: types.BoolValue(false),
		TicketAutoClose: types.BoolValue(false)}
	prior := alertProfileModel{ID: types.StringValue("ap1"), Name: types.StringValue("Ops"),
		Channels: []alertProfileChannelModel{slack, ch("Telegram", "chat-a", "tok-a"), ch("Telegram", "chat-b", "tok-b")}}

	got, _ := tfRead(t, f, NewAlertProfileResource(), prior)
	want := []struct{ typ, recipient, token string }{{"Slack", "", ""}, {"Telegram", "chat-a", "tok-a"}, {"Telegram", "chat-b", "tok-b"}}
	for i, w := range want {
		c := got.Channels[i]
		if c.Type.ValueString() != w.typ || c.BotRecipient.ValueString() != w.recipient || c.BotToken.ValueString() != w.token {
			t.Fatalf("channel %d = %s/%s/%s, want %v", i, c.Type, c.BotRecipient, c.BotToken, w)
		}
	}
	if got.Channels[0].SlackSigningSecret.ValueString() != "sign" {
		t.Fatal("slack_signing_secret must stay on the Slack channel")
	}
}
