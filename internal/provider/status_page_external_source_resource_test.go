package provider

import "testing"

// A component has at most one external source, and every endpoint addresses it
// by component rather than by the source row's own id. Read lists the page's
// sources, so it has to match on component_id: matching on id would lose the row
// after somebody re-created the source in the portal, and the provider would
// then plan a create on top of one that already exists.
func TestFindStatusPageExternalSourceMatchesOnComponent(t *testing.T) {
	sources := []statusPageExternalSourceResponse{
		{ID: "es_1", ComponentID: "c_1", URL: "https://status.upstream.example/api/v2/summary.json"},
		{ID: "es_2", ComponentID: "c_2", URL: "https://status.other.example/health.json"},
	}
	got := findStatusPageExternalSource(sources, "c_2")
	if got == nil || got.ID != "es_2" {
		t.Fatalf("expected es_2 for component c_2, got %v", got)
	}
	if got := findStatusPageExternalSource(sources, "c_3"); got != nil {
		t.Fatalf("expected no match for a component with no source, got %v", got)
	}
}

// The API drops json_path for a StatuspageSummary source, so it comes back null
// whatever was configured. applyResponse has to take the response verbatim,
// because a plain Optional attribute that keeps a value the server discarded
// fails the apply.
func TestStatusPageExternalSourceApplyResponseTakesTheServerShape(t *testing.T) {
	m := statusPageExternalSourceModel{}
	m.applyResponse(&statusPageExternalSourceResponse{
		ID: "es_1", ComponentID: "c_1", StatusPageID: "sp_1",
		Kind: "StatuspageSummary", URL: "https://status.upstream.example/api/v2/summary.json",
		JSONPath: nil, PollIntervalSeconds: 300, IsEnabled: true, EffectiveHealth: "Unknown",
	})

	if !m.JSONPath.IsNull() {
		t.Fatalf("json_path should be null for a StatuspageSummary source, got %v", m.JSONPath)
	}
	if m.PollIntervalSecond.ValueInt64() != 300 || !m.IsEnabled.ValueBool() {
		t.Fatalf("defaults not read back: %v / %v", m.PollIntervalSecond, m.IsEnabled)
	}
	if m.EffectiveHealth.ValueString() != "Unknown" {
		t.Fatalf("effective_health not read: %v", m.EffectiveHealth)
	}
}
