package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// MFA and the timeout go to the tenant PATCH, the ranges to the validating
// /ip-allowlist PATCH as CSV, and emptying the set must send "" (clear) rather
// than omit the field and leave the old ranges enforced.
func TestTenantSecuritySettingsSplitsAcrossEndpoints(t *testing.T) {
	f := newFakeAPI(t)
	tenant := "/identity/tenants/" + testTenant
	ranges := "203.0.113.0/24,2001:db8::/32"
	f.on("PATCH "+tenant, ok(map[string]any{}))
	f.on("PATCH "+tenant+"/ip-allowlist", ok(map[string]any{}))
	f.on("GET "+tenant, func(recordedRequest) (int, any) {
		return 200, map[string]any{"enforceMfa": true, "sessionTimeoutMinutes": 30,
			"ipAllowlistEnabled": true, "allowedIpRanges": ranges}
	})

	set, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"203.0.113.0/24", "2001:db8::/32"})
	got := tfCreate(t, f, NewTenantSecuritySettingsResource(), tenantSecuritySettingsModel{
		EnforceMfa: types.BoolValue(true), SessionTimeoutMinutes: types.Int64Value(30),
		IPAllowlistEnabled: types.BoolValue(true), AllowedIPRanges: set,
	})
	if got.ID.ValueString() != testTenant || !got.EnforceMfa.ValueBool() || len(got.AllowedIPRanges.Elements()) != 2 {
		t.Fatalf("state = %+v", got)
	}
	tp := f.last("PATCH " + tenant).Body
	if tp["enforceMfa"] != true || tp["sessionTimeoutMinutes"].(float64) != 30 {
		t.Fatalf("tenant PATCH = %v", tp)
	}
	if _, leaked := tp["allowedIpRanges"]; leaked {
		t.Fatal("ranges must go through the validating /ip-allowlist endpoint, not the tenant PATCH")
	}
	mustContain(t, f.last("PATCH "+tenant+"/ip-allowlist").Raw, "203.0.113.0/24")

	cleared := got
	cleared.AllowedIPRanges = types.SetNull(types.StringType)
	ranges = ""
	tfUpdate(t, f, NewTenantSecuritySettingsResource(), got, cleared)
	if v, present := f.last("PATCH " + tenant + "/ip-allowlist").Body["allowedIpRanges"]; !present || v != "" {
		t.Fatalf("allowedIpRanges = %v, want \"\" to clear", v)
	}

	// Destroy must not touch the tenant: no request at all.
	before := len(f.requests)
	tfDelete(t, f, NewTenantSecuritySettingsResource(), got)
	if len(f.requests) != before {
		t.Fatal("destroy must leave the security settings in force")
	}
}
