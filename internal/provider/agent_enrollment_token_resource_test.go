package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Create posts the mint body and lands the one-time token and install commands
// in state; Read keeps state without calling the API (a spent token must not be
// recreated); Delete revokes, and a 404 on revoke is not an error.
func TestAgentEnrollmentTokenLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/regions", "/agents/enrollment-tokens")
	item := list + "/tok1"
	f.on("POST "+list, ok(map[string]any{
		"tokenId": "tok1", "token": "raw_secret", "expiresAt": "2026-10-19T00:00:00Z",
		"maxUses": 3, "remainingUses": 3, "roles": []string{"private-location"},
		"autoUpdate": false, "locationId": nil,
		"installCommands": map[string]any{"linux": "curl ... --token raw_secret", "windows": "irm ...",
			"macos": "curl ...", "helm": nil},
	}))
	f.on("DELETE "+item, ok(map[string]any{"revoked": true}))

	ctx := context.Background()
	roles, _ := types.SetValueFrom(ctx, types.StringType, []string{"private-location"})
	got := tfCreate(t, f, NewAgentEnrollmentTokenResource(), agentEnrollmentTokenModel{
		Roles: roles, LocationName: types.StringValue("dc-east"),
		MaxUses: types.Int64Value(3), AutoUpdate: types.BoolValue(false),
		ExpiresInHours: types.Int64Null(), LocationID: types.StringNull(),
		Token: types.StringUnknown(), ExpiresAt: types.StringUnknown(), ID: types.StringUnknown(),
		InstallCommands: types.MapUnknown(types.StringType),
	})

	body := f.last("POST " + list).Body
	if body["locationName"] != "dc-east" || body["maxUses"] != float64(3) || body["autoUpdate"] != false {
		t.Fatalf("mint body = %v", body)
	}
	if _, present := body["expiresInHours"]; present {
		t.Fatal("an omitted expiry must be left to the server default")
	}
	if _, present := body["locationId"]; present {
		t.Fatal("an omitted location_id must not be sent")
	}
	if got.ID.ValueString() != "tok1" || got.Token.ValueString() != "raw_secret" {
		t.Fatalf("state = %+v", got)
	}
	cmds := got.InstallCommands.Elements()
	if len(cmds) != 3 {
		t.Fatalf("install_commands = %v, want linux/windows/macos with null helm dropped", cmds)
	}

	read, removed := tfRead(t, f, NewAgentEnrollmentTokenResource(), got)
	if removed || read.Token.ValueString() != "raw_secret" {
		t.Fatal("Read must keep the created token in state")
	}
	if f.count("GET "+list) != 0 {
		t.Fatal("Read must not consult the list endpoint")
	}

	tfDelete(t, f, NewAgentEnrollmentTokenResource(), got)
	if f.count("DELETE "+item) != 1 {
		t.Fatal("Delete must revoke the token")
	}

	f.on("DELETE "+item, func(recordedRequest) (int, any) { return http.StatusNotFound, nil })
	tfDelete(t, f, NewAgentEnrollmentTokenResource(), got)
}
