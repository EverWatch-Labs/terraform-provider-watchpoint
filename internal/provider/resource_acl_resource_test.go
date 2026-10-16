package provider

import (
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func aclRow(id, kind, pattern, userID, roleID string) resourceAclRow {
	row := resourceAclRow{ID: id, ResourceKind: kind, Pattern: pattern}
	if userID != "" {
		row.UserID = &userID
	}
	if roleID != "" {
		row.RoleID = &roleID
	}
	return row
}

// The API returns the tenant's WHOLE access-control listing with no filters, so
// picking the wrong rows out of it would make one grant delete another subject's
// access. Kind and subject id are matched case-insensitively because the API
// lower-cases the kind on write and resource ids vary in casing by producer.
func TestMatchResourceAclRowsPicksOneSubjectAndKind(t *testing.T) {
	rows := []resourceAclRow{
		aclRow("a1", "monitor", "mon_1", "", "role_payments"),
		aclRow("a2", "monitor", "mon_2", "", "role_payments"),
		aclRow("a3", "monitor", "mon_9", "", "role_search"),
		aclRow("a4", "dashboard", "dash_1", "", "role_payments"),
		aclRow("a5", "monitor", "mon_7", "user_payments", ""),
	}

	got := matchResourceAclRows(rows, "MONITOR", "role", "ROLE_PAYMENTS")
	if len(got) != 2 || got[0].ID != "a1" || got[1].ID != "a2" {
		t.Fatalf("expected the two payments-role monitor rows, got %v", got)
	}

	// A member grant must never be picked up by a role grant of the same kind,
	// and vice versa.
	if got := matchResourceAclRows(rows, "monitor", "user", "role_payments"); len(got) != 0 {
		t.Fatalf("a role id matched a member grant: %v", got)
	}
	if got := matchResourceAclRows(rows, "monitor", "user", "user_payments"); len(got) != 1 || got[0].ID != "a5" {
		t.Fatalf("expected only the member row, got %v", got)
	}
	if got := matchResourceAclRows(rows, "rum_site", "role", "role_payments"); len(got) != 0 {
		t.Fatalf("expected no rows for an ungranted kind, got %v", got)
	}
}

// The API trims a pattern on write, so writing the stored spelling back into the
// Required patterns set fails the apply with "was cty.StringVal(\"MON_1\"), but
// now cty.StringVal(\"mon_1\")". A genuinely different value is real drift and
// must land in state so the next plan proposes putting it back.
func TestAclPatternsFromRowsKeepsConfiguredSpelling(t *testing.T) {
	configured := []string{"MON_1", " mon_2 "}
	rows := []resourceAclRow{
		aclRow("a1", "monitor", "mon_1", "", "r1"),
		aclRow("a2", "monitor", "mon_2", "", "r1"),
		aclRow("a3", "monitor", "mon_added_in_the_portal", "", "r1"),
	}

	got := aclPatternsFromRows(configured, rows)
	sort.Strings(got)
	want := []string{"MON_1", " mon_2 ", "mon_added_in_the_portal"}
	sort.Strings(want)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("patterns %v, want %v", got, want)
		}
	}
}

// The diff drives revoke-and-regrant against an API with no update endpoint. A
// pattern that only changed spelling must NOT round-trip through a delete, and a
// row the configuration no longer names must be revoked.
func TestAclPatternDiffAddsAndRemoves(t *testing.T) {
	rows := []resourceAclRow{
		aclRow("a1", "monitor", "mon_1", "", "r1"),
		aclRow("a2", "monitor", "mon_stale", "", "r1"),
	}

	add, removeIDs := aclPatternDiff([]string{"MON_1", "mon_new", " mon_new "}, rows)

	if len(add) != 1 || add[0] != "mon_new" {
		t.Fatalf("expected one create for mon_new (duplicate spellings collapse), got %v", add)
	}
	if len(removeIDs) != 1 || removeIDs[0] != "a2" {
		t.Fatalf("expected only the stale row revoked, got %v", removeIDs)
	}

	// An unchanged set writes nothing at all.
	add, removeIDs = aclPatternDiff([]string{"mon_1", "mon_stale"}, rows)
	if len(add) != 0 || len(removeIDs) != 0 {
		t.Fatalf("unchanged set produced writes: add=%v remove=%v", add, removeIDs)
	}
}

// The synthetic id is what import parses back, so the two subject shapes must
// round-trip through the same three-part form.
func TestResourceAclSyntheticID(t *testing.T) {
	m := resourceAclModel{}
	m.ResourceKind = types.StringValue("monitor")
	m.RoleID = types.StringValue("role_1")
	if got := m.syntheticID(); got != "monitor/role/role_1" {
		t.Fatalf("role id: %q", got)
	}

	m = resourceAclModel{}
	m.ResourceKind = types.StringValue("rum_site")
	m.UserID = types.StringValue("user_1")
	if got := m.syntheticID(); got != "rum_site/user/user_1" {
		t.Fatalf("user id: %q", got)
	}
}
