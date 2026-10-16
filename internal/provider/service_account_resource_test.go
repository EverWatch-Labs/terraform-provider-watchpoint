package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestServiceAccountLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/identity", "/service-accounts")
	item := list + "/sa1"
	desc := "CI deployer"
	row := map[string]any{"id": "sa1", "name": "ci", "description": desc, "roleId": "r1", "roleName": "Deployer",
		"isActive": true, "createdAt": "2026-10-01T00:00:00Z", "activeKeyCount": 0}
	f.on("POST "+list, func(recordedRequest) (int, any) { return http.StatusCreated, row })
	f.on("GET "+list, func(recordedRequest) (int, any) { return http.StatusOK, []any{row} })
	f.on("PATCH "+item, ok(map[string]any{"id": "sa1"}))
	f.on("DELETE "+item, ok(map[string]any{"revokedKeys": 2}))

	got := tfCreate(t, f, NewServiceAccountResource(), serviceAccountModel{
		Name: types.StringValue("ci"), Description: types.StringValue(desc),
		RoleID: types.StringValue("r1"), IsActive: types.BoolValue(true),
	})
	if got.RoleName.ValueString() != "Deployer" || got.ActiveKeyCount.ValueInt64() != 0 {
		t.Fatalf("state = %+v", got)
	}
	if f.count("PATCH "+item) != 0 {
		t.Fatal("an active account needs no follow-up PATCH")
	}

	// Clearing the description must send "", which the API stores as null.
	plan := got
	plan.Description = types.StringNull()
	row["description"] = nil
	after := tfUpdate(t, f, NewServiceAccountResource(), got, plan)
	if v, present := f.last("PATCH " + item).Body["description"]; !present || v != "" {
		t.Fatalf("description = %v, want \"\"", v)
	}
	if !after.Description.IsNull() {
		t.Fatal("cleared description must read back null")
	}
	tfDelete(t, f, NewServiceAccountResource(), after)
}

// A service-account key carries the account id and its own CIDR allowlist on
// create, and both read back from the key list.
func TestAPIKeyCarriesServiceAccountAndIPRanges(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/identity", "/api-keys")
	ranges := "10.0.0.0/8,192.0.2.7"
	f.on("POST "+list, ok(map[string]any{"id": "k1", "name": "ci", "keyPrefix": "wp_live_abc...",
		"scopes": "monitors.read", "createdAt": "2026-10-01T00:00:00Z", "apiKey": "wp_live_secret"}))
	f.on("GET "+list, ok([]any{map[string]any{"id": "k1", "name": "ci", "keyPrefix": "wp_live_abc...",
		"scopes": "monitors.read", "isActive": true, "createdAt": "2026-10-01T00:00:00Z",
		"allowedIpRanges": ranges, "serviceAccountId": "sa1"}}))

	ctx := context.Background()
	scopes, _ := types.SetValueFrom(ctx, types.StringType, []string{"monitors.read"})
	ips, _ := types.SetValueFrom(ctx, types.StringType, []string{"10.0.0.0/8", "192.0.2.7"})
	got := tfCreate(t, f, NewAPIKeyResource(), apiKeyModel{
		Name: types.StringValue("ci"), Scopes: scopes, Enabled: types.BoolValue(true),
		AllowedIPRanges: ips, ServiceAccountID: types.StringValue("sa1"),
	})
	body := f.last("POST " + list).Body
	if body["serviceAccountId"] != "sa1" {
		t.Fatalf("serviceAccountId = %v", body["serviceAccountId"])
	}
	mustContain(t, body["allowedIpRanges"].(string), "10.0.0.0/8")
	if got.APIKey.ValueString() != "wp_live_secret" {
		t.Fatal("the plaintext key must land in state")
	}

	read, _ := tfRead(t, f, NewAPIKeyResource(), got)
	if len(read.AllowedIPRanges.Elements()) != 2 || read.ServiceAccountID.ValueString() != "sa1" {
		t.Fatalf("read = %+v", read)
	}
}
