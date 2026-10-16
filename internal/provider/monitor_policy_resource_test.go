package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A per-monitor "skipped" result arrives inside a 200 and must still fail the
// apply, and destroy resets confirmation and clears the owner.
func TestMonitorPolicyWritesThroughBulkEndpoint(t *testing.T) {
	f := newFakeAPI(t)
	bulk := tenantPath("/monitoring", "/monitors/bulk")
	status := "updated"
	f.on("PATCH "+bulk, func(recordedRequest) (int, any) {
		return 200, map[string]any{"results": []any{map[string]any{"id": "m1", "status": status, "error": "private location"}}}
	})

	got := tfCreate(t, f, NewMonitorPolicyResource(), monitorPolicyModel{
		MonitorID: types.StringValue("m1"), ConfirmationPolicy: types.StringValue("Quorum"),
		QuorumMinRegions: types.Int64Value(3), OwnerGroupID: types.StringValue("g1"),
	})
	body := f.last("PATCH " + bulk).Body
	if body["confirmationPolicy"] != "Quorum" || body["quorumMinRegions"].(float64) != 3 || body["ownerGroupId"] != "g1" {
		t.Fatalf("bulk body = %v", body)
	}
	if ids := body["monitorIds"].([]any); len(ids) != 1 || ids[0] != "m1" {
		t.Fatalf("monitorIds = %v", ids)
	}
	if got.ID.ValueString() != "m1" {
		t.Fatal("id must equal monitor_id")
	}

	tfDelete(t, f, NewMonitorPolicyResource(), got)
	reset := f.last("PATCH " + bulk).Body
	if reset["confirmationPolicy"] != "None" || reset["ownerGroupId"] != "" {
		t.Fatalf("destroy body = %v", reset)
	}

	status = "skipped"
	r := configured(t, f, NewMonitorPolicyResource()).(*monitorPolicyResource)
	if err := r.write(t.Context(), (&got).toBody()); err == nil {
		t.Fatal("a skipped monitor must be an error")
	}
}
