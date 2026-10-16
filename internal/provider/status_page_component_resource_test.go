package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// There is no GET for a single component, so Read finds it in the owning page's
// detail projection. Matching the wrong row, or failing to notice the component
// is gone, is the difference between a clean plan and a silent recreate.
func TestFindStatusPageComponentPicksTheRightRow(t *testing.T) {
	components := []statusPageComponentResponse{
		{ID: "c_1", Name: "API"},
		{ID: "c_2", Name: "Dashboard"},
	}
	if got := findStatusPageComponent(components, "c_2"); got == nil || got.Name != "Dashboard" {
		t.Fatalf("expected the Dashboard row, got %v", got)
	}
	if got := findStatusPageComponent(components, "c_3"); got != nil {
		t.Fatalf("expected no match for a deleted component, got %v", got)
	}
}

// The API returns `[]` for a membership the configuration simply omits. Writing
// that back as a known empty Set fails the apply with "was null, but now
// cty.SetVal([])", so an empty array has to become a null Set.
func TestStatusPageComponentApplyResponseMapsEmptyMembershipToNull(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	ds := &diagSink{d: &diags}

	var m statusPageComponentModel
	m.applyResponse(ctx, &statusPageComponentResponse{
		ID: "c_1", StatusPageID: "sp_1", Name: "API",
		MonitorIDs: []string{}, DependsOn: nil,
		AggregationMode: "WorstOf", Position: 3,
	}, ds)

	if !m.MonitorIDs.IsNull() {
		t.Fatalf("an empty monitor array should be a null set, got %v", m.MonitorIDs)
	}
	if !m.DependsOn.IsNull() {
		t.Fatalf("a missing dependency array should be a null set, got %v", m.DependsOn)
	}
	if m.Position.ValueInt64() != 3 {
		t.Fatalf("position not taken from the response: %v", m.Position)
	}

	m.applyResponse(ctx, &statusPageComponentResponse{
		ID: "c_1", MonitorIDs: []string{"mon_1", "mon_2"}, AggregationMode: "Majority",
	}, ds)
	if len(m.MonitorIDs.Elements()) != 2 {
		t.Fatalf("a real membership did not land in state: %v", m.MonitorIDs)
	}
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
}

// The monitors and dependencies PUT bodies are dereferenced by the API without a
// null check, so a null array is a 500 rather than the clear the caller meant.
func TestEmptyIfNilNeverSendsANullArray(t *testing.T) {
	if got := emptyIfNil(nil); got == nil || len(got) != 0 {
		t.Fatalf("expected an empty slice, got %v", got)
	}
	if got := emptyIfNil([]string{"mon_1"}); len(got) != 1 {
		t.Fatalf("expected the original slice, got %v", got)
	}
}
