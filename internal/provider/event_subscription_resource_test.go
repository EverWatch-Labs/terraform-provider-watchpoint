package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The secret only comes back on create and rotate. It must survive every later
// read, rotate only when rotate_trigger changes, and a paused subscription has
// to be paused straight after create because create always starts active.
func TestEventSubscriptionLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/identity", "/event-subscriptions")
	item := list + "/es1"
	row := map[string]any{"id": "es1", "name": "SIEM", "url": "https://siem.example.com/in",
		"isActive": false, "eventTypes": []string{"monitor.down", "incident.opened"}, "createdAt": "2026-10-01T00:00:00Z"}
	f.on("POST "+list, ok(map[string]any{"id": "es1", "signingSecret": "sec-1"}))
	f.on("GET "+list, ok([]any{row}))
	f.on("PATCH "+item, ok(map[string]any{"id": "es1"}))
	f.on("POST "+item+"/rotate-secret", ok(map[string]any{"signingSecret": "sec-2"}))
	f.on("DELETE "+item, ok(map[string]any{"success": true}))

	ctx := context.Background()
	types1, _ := types.SetValueFrom(ctx, types.StringType, []string{"incident.opened", "monitor.down"})
	plan := eventSubscriptionModel{
		Name: types.StringValue("SIEM"), URL: types.StringValue("https://siem.example.com/in"),
		EventTypes: types1, IsActive: types.BoolValue(false),
	}
	got := tfCreate(t, f, NewEventSubscriptionResource(), plan)
	if got.SigningSecret.ValueString() != "sec-1" {
		t.Fatalf("signing_secret = %q", got.SigningSecret.ValueString())
	}
	if f.last("PATCH " + item).Body["isActive"] != false {
		t.Fatal("is_active = false must be applied with a PATCH after create")
	}

	read, _ := tfRead(t, f, NewEventSubscriptionResource(), got)
	if read.SigningSecret.ValueString() != "sec-1" {
		t.Fatal("a read must not lose the secret the list never returns")
	}

	same := tfUpdate(t, f, NewEventSubscriptionResource(), read, read)
	if f.count("POST "+item+"/rotate-secret") != 0 || same.SigningSecret.ValueString() != "sec-1" {
		t.Fatal("an unchanged rotate_trigger must not rotate")
	}
	rotate := read
	rotate.RotateTrigger = types.StringValue("2026-10")
	rotated := tfUpdate(t, f, NewEventSubscriptionResource(), read, rotate)
	if rotated.SigningSecret.ValueString() != "sec-2" {
		t.Fatalf("signing_secret after rotation = %q, want sec-2", rotated.SigningSecret.ValueString())
	}

	tfDelete(t, f, NewEventSubscriptionResource(), rotated)
}

func TestEventSubscriptionReadDropsMissing(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET "+tenantPath("/identity", "/event-subscriptions"), func(recordedRequest) (int, any) {
		return http.StatusOK, []any{}
	})
	_, removed := tfRead(t, f, NewEventSubscriptionResource(), eventSubscriptionModel{ID: types.StringValue("es9"), EventTypes: types.SetNull(types.StringType)})
	if !removed {
		t.Fatal("a subscription missing from the list must leave state")
	}
}
