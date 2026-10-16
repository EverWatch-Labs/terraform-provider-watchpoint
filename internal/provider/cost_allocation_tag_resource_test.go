package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The billing API lower-cases a tag key on write, so a listing row for a config
// that says "Team" comes back as "team". Matching on the raw string loses the
// row and the provider then deletes and recreates a mapping that never moved.
func TestFindCostTagRowMatchesTheNormalisedKey(t *testing.T) {
	rows := []costTagRow{
		{ID: "ct_1", TagKey: "env", TagValue: "prod", ResourceType: "Monitor", ResourceID: "mon_1"},
		{ID: "ct_2", TagKey: "team", TagValue: "payments", ResourceType: "Monitor", ResourceID: "mon_1"},
		{ID: "ct_3", TagKey: "team", TagValue: "search", ResourceType: "Synthetic", ResourceID: "mon_1"},
	}

	got := findCostTagRow(rows, "Team", "Monitor", "mon_1")
	if got == nil || got.ID != "ct_2" {
		t.Fatalf("expected ct_2 for the Monitor row under key Team, got %v", got)
	}

	// Resource type is part of the identity: same key, same resource id, other
	// category is a different mapping.
	if got := findCostTagRow(rows, "team", "Storage", "mon_1"); got != nil {
		t.Fatalf("expected no match for an unmapped category, got %v", got)
	}
	if got := findCostTagRow(rows, "team", "Monitor", "mon_2"); got != nil {
		t.Fatalf("expected no match for an untagged resource, got %v", got)
	}
}

// applyRow must not write the stored spelling of tag_key back into state: a
// Required attribute whose applied value differs from config fails the apply
// with "was cty.StringVal(\"Team\"), but now cty.StringVal(\"team\")".
func TestCostTagApplyRowKeepsTheConfiguredKeySpelling(t *testing.T) {
	m := costAllocationTagModel{
		TagKey:       types.StringValue("Team"),
		TagValue:     types.StringValue("payments"),
		ResourceType: types.StringValue("Monitor"),
		ResourceID:   types.StringValue("mon_1"),
	}
	m.applyRow(&costTagRow{ID: "ct_2", TagKey: "team", TagValue: "payments",
		ResourceType: "Monitor", ResourceID: "mon_1"})

	if m.TagKey.ValueString() != "Team" {
		t.Fatalf("tag_key was rewritten to %q", m.TagKey.ValueString())
	}
	if m.TagValue.ValueString() != "payments" {
		t.Fatalf("tag_value was rewritten to %q", m.TagValue.ValueString())
	}
	if m.ID.ValueString() != "ct_2" {
		t.Fatalf("id not taken from the row: %q", m.ID.ValueString())
	}
}

// A value changed outside Terraform is real drift and must land in state so the
// next plan proposes putting it back.
func TestCostTagApplyRowTakesAChangedValue(t *testing.T) {
	m := costAllocationTagModel{TagValue: types.StringValue("payments")}
	m.applyRow(&costTagRow{ID: "ct_2", TagValue: "search"})
	if m.TagValue.ValueString() != "search" {
		t.Fatalf("expected the server value, got %q", m.TagValue.ValueString())
	}
}
