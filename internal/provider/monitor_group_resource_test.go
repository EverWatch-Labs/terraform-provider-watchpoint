package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Group tags are the cascade that gives every member monitor its cost-allocation
// key, so the CSV <-> set mapping has to survive both directions. An absent tag
// field must map to a null set, not an empty one: an empty set would diff forever
// against a config that simply omits the attribute, which is the
// "Provider produced inconsistent result after apply" failure.
func TestMonitorGroupTagsMapping(t *testing.T) {
	var diags diag.Diagnostics
	ctx := context.Background()
	csv := "team:payments,prod"

	var m monitorGroupModel
	m.applyResponse(ctx, &monitorGroupResponse{ID: "grp_1", Name: "Payments", Tags: &csv}, &diags)
	if len(m.Tags.Elements()) != 2 {
		t.Fatalf("len(Tags) = %d, want 2", len(m.Tags.Elements()))
	}

	empty := ""
	for name, in := range map[string]*string{"nil": nil, "empty": &empty} {
		var g monitorGroupModel
		g.applyResponse(ctx, &monitorGroupResponse{ID: "grp_1", Name: "Payments", Tags: in}, &diags)
		if !g.Tags.IsNull() {
			t.Fatalf("Tags from %s response = %v, want null", name, g.Tags)
		}
	}

	// Create omits an unset field; the PATCH path must send "" instead so a
	// removed tag list actually clears server-side.
	if body := (&monitorGroupModel{Tags: types.SetNull(types.StringType)}).toBody(ctx, &diags); body.Tags != nil {
		t.Fatalf("create body Tags = %q, want omitted", *body.Tags)
	}
	if got := csvPtrClearing(ctx, types.SetNull(types.StringType), &diags); got == nil || *got != "" {
		t.Fatalf("update body Tags = %v, want a pointer to the empty string", got)
	}
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

// Unordered membership has to be a Set. As a List, Terraform compares the
// element order it sent against the order the API returned and the apply dies
// with "Provider produced inconsistent result after apply" - which is why the
// repo-wide TestNoFlatListAttributes exists. Pinned here too because tags is
// the attribute most likely to be "simplified" back to a list.
func TestMonitorGroupTagsIsAnUnorderedSetOfStrings(t *testing.T) {
	var resp resource.SchemaResponse
	NewMonitorGroupResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["tags"]
	if !ok {
		t.Fatal("watchpoint_monitor_group lost its tags attribute; a group tag is the whole cascade")
	}
	set, ok := attr.(schema.SetAttribute)
	if !ok {
		t.Fatalf("tags is %T, want schema.SetAttribute", attr)
	}
	if set.ElementType != types.StringType {
		t.Fatalf("tags element type = %v, want string", set.ElementType)
	}
	if !set.Optional {
		t.Error("tags must be optional; a group without tags is the normal case")
	}
}

// The comma string is the wire shape the monitoring API takes for tags, on
// groups and on monitors alike, so both tag shapes have to survive it untouched:
// a key:value pair is what makes spend groupable, a bare label is still a tag.
func TestMonitorGroupTagsRoundTripBothShapes(t *testing.T) {
	var diags diag.Diagnostics
	ctx := context.Background()

	var m monitorGroupModel
	csv := "team:payments,prod"
	m.applyResponse(ctx, &monitorGroupResponse{ID: "grp_1", Name: "Payments", Tags: &csv}, &diags)

	body := m.toBody(ctx, &diags)
	if body.Tags == nil || *body.Tags != csv {
		t.Fatalf("round trip = %v, want %q", body.Tags, csv)
	}
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}
