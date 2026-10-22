package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Only configured halves are sent (each needs its own feature), an empty set
// turns approvals off, and destroy resets only what is on.
func TestTenantMonitoringPolicy(t *testing.T) {
	f := newFakeAPI(t)
	p := tenantPath("/monitoring", "/monitoring-policy")
	cur := map[string]any{"lockTerraformManaged": false, "lockAvailable": false, "approvalEnvironmentIds": []any{"env_p"},
		"approvalMinApprovers": 2, "approvalAllowSelf": false, "approvalsAvailable": true}
	f.on("PUT "+p, ok(cur))
	f.on("GET "+p, ok(cur))

	plan := tenantMonitoringPolicyModel{
		ApprovalEnvironmentIDs: strSet("env_p"), ApprovalMinApprovers: types.Int64Value(2),
	}
	got := tfCreate(t, f, NewTenantMonitoringPolicyResource(), plan)
	b := f.last("PUT " + p).Body
	if _, sent := b["lockTerraformManaged"]; sent || b["approvalMinApprovers"] != float64(2) {
		t.Fatalf("put = %v", b)
	}
	if got.ID.ValueString() != testTenant || got.LockTerraformManaged.ValueBool() || !got.ApprovalsAvailable.ValueBool() {
		t.Fatalf("state = %+v", got)
	}

	next := got
	next.ApprovalEnvironmentIDs = strSet()
	next.LockTerraformManaged, next.ApprovalMinApprovers, next.ApprovalAllowSelf = types.BoolNull(), types.Int64Null(), types.BoolNull()
	tfUpdate(t, f, NewTenantMonitoringPolicyResource(), got, next)
	if raw := f.last("PUT " + p).Raw; !strings.Contains(raw, `"approvalEnvironmentIds":[]`) {
		t.Fatalf("empty set must be sent as []: %s", raw)
	}

	tfDelete(t, f, NewTenantMonitoringPolicyResource(), got)
	if raw := f.last("PUT " + p).Raw; strings.Contains(raw, "lockTerraformManaged") || !strings.Contains(raw, `"approvalEnvironmentIds":[]`) {
		t.Fatalf("delete body = %s", raw)
	}
}
