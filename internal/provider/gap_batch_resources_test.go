package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A metric-ratio SLO sends its kind and config, keeps the operator's JSON
// spelling when the API re-serialises it, and clearing the config sends "".
func TestSLAProfileTelemetrySource(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/monitoring", "/sla-profiles")
	item := list + "/sla1"
	row := map[string]any{"id": "sla1", "name": "Checkout", "targetUptimePercent": 99.9, "windowDays": 30,
		"atRiskThresholdPercent": 75.0, "excludeMaintenanceWindows": true, "alertOnBreach": true,
		"sourceKind": "Metrics", "sourceConfigJson": `{"totalMetric":"http.requests","goodMetric":"http.ok"}`,
		"createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-01T00:00:00Z"}
	f.on("POST "+list, func(recordedRequest) (int, any) { return http.StatusCreated, row })
	f.on("GET "+item, func(recordedRequest) (int, any) { return http.StatusOK, row })
	f.on("PATCH "+item, func(recordedRequest) (int, any) { return http.StatusNoContent, nil })

	cfg := `{"goodMetric": "http.ok", "totalMetric": "http.requests"}`
	plan := slaProfileModel{
		Name: types.StringValue("Checkout"), TargetUptimePercent: types.Float64Value(99.9), WindowDays: types.Int64Value(30),
		AtRiskThresholdPercent: types.Float64Value(75), ExcludeMaintenanceWindows: types.BoolValue(true),
		AlertOnBreach: types.BoolValue(true), AlertOnAtRisk: types.BoolValue(false), IsDefault: types.BoolValue(false),
		SourceKind: types.StringValue("Metrics"), SourceConfigJSON: types.StringValue(cfg),
	}
	got := tfCreate(t, f, NewSLAProfileResource(), plan)
	body := f.last("POST " + list).Body
	if body["sourceKind"] != "Metrics" || body["sourceConfigJson"] != cfg {
		t.Fatalf("body = %v", body)
	}
	if got.SourceConfigJSON.ValueString() != cfg {
		t.Fatalf("config spelling not kept: %q", got.SourceConfigJSON.ValueString())
	}

	next := got
	next.SourceKind, next.SourceConfigJSON = types.StringValue("Monitor"), types.StringNull()
	row["sourceKind"], row["sourceConfigJson"] = "Monitor", nil
	after := tfUpdate(t, f, NewSLAProfileResource(), got, next)
	patch := f.last("PATCH " + item).Body
	if v, present := patch["sourceConfigJson"]; !present || v != "" || patch["sourceKind"] != "Monitor" {
		t.Fatalf("patch = %v", patch)
	}
	if !after.SourceConfigJSON.IsNull() || after.SourceKind.ValueString() != "Monitor" {
		t.Fatalf("state = %+v", after)
	}
}

func TestDashboardFolderLifecycle(t *testing.T) {
	f := newFakeAPI(t)
	list := tenantPath("/analytics", "/dashboard-folders")
	item := list + "/df1"
	row := map[string]any{"id": "df1", "name": "Payments", "parentId": "top", "dashboardCount": 0, "createdAt": "2026-10-01T00:00:00Z"}
	f.on("POST "+list, func(recordedRequest) (int, any) { return http.StatusCreated, row })
	f.on("GET "+list, func(recordedRequest) (int, any) { return http.StatusOK, []any{row} })
	f.on("PATCH "+item, func(recordedRequest) (int, any) { return http.StatusOK, row })
	f.on("DELETE "+item, func(recordedRequest) (int, any) { return http.StatusNoContent, nil })

	got := tfCreate(t, f, NewDashboardFolderResource(), dashboardFolderModel{
		Name: types.StringValue("Payments"), ParentID: types.StringValue("top"),
	})
	if f.last("POST " + list).Body["parentId"] != "top" || got.ParentID.ValueString() != "top" {
		t.Fatalf("state = %+v", got)
	}

	// Un-nesting must send parentId "" (move to root); null would leave it nested.
	next := got
	next.ParentID = types.StringNull()
	row["parentId"] = nil
	after := tfUpdate(t, f, NewDashboardFolderResource(), got, next)
	if v, present := f.last("PATCH " + item).Body["parentId"]; !present || v != "" {
		t.Fatalf("parentId = %v, want \"\"", v)
	}
	if !after.ParentID.IsNull() {
		t.Fatal("root folder must read back with a null parent")
	}

	// A folder deleted outside Terraform drops out of state.
	f.on("GET "+list, ok([]any{}))
	if _, removed := tfRead(t, f, NewDashboardFolderResource(), after); !removed {
		t.Fatal("a folder missing from the list must be removed from state")
	}
	tfDelete(t, f, NewDashboardFolderResource(), after)
}

// Moving a dashboard back to the root and clearing its owner both need "".
func TestDashboardFolderAndOwnerClear(t *testing.T) {
	m := dashboardModel{Name: types.StringValue("d"), FolderID: types.StringNull(), OwnerGroupID: types.StringNull()}
	if c := m.toCreateBody(); c.FolderID != nil || c.OwnerGroupID != nil {
		t.Fatal("create must omit unset folder and owner")
	}
	u := m.toUpdateBody()
	if u.FolderID == nil || *u.FolderID != "" || u.OwnerGroupID == nil || *u.OwnerGroupID != "" {
		t.Fatalf("update = %+v, want \"\" for both", u)
	}
}

// Owner groups on routing and SGL rules: set on create, cleared with "" on update.
func TestRuleOwnerGroups(t *testing.T) {
	routing := alertRoutingRuleModel{Name: types.StringValue("r"), OwnerGroupID: types.StringNull()}
	if b := routing.toBody(context.Background(), &diagSink{}); b.OwnerGroupID == nil || *b.OwnerGroupID != "" {
		t.Fatal("routing rule must always send ownerGroupId so removing it clears it")
	}
	sgl := sglAlertRuleModel{Name: types.StringValue("s"), OwnerGroupID: types.StringValue("g1")}
	if c := sgl.toCreateBody(); c.OwnerGroupID == nil || *c.OwnerGroupID != "g1" {
		t.Fatal("sgl create must carry the owner")
	}
	sgl.OwnerGroupID = types.StringNull()
	if u := sgl.toUpdateBody(); u.OwnerGroupID == nil || *u.OwnerGroupID != "" {
		t.Fatal("sgl update must clear a removed owner with \"\"")
	}
	var applied sglAlertRuleModel
	g := "g2"
	applied.applyResponse(&sglAlertRuleResponse{ID: "s1", OwnerGroupID: &g})
	if applied.OwnerGroupID.ValueString() != "g2" {
		t.Fatal("sgl owner not read back")
	}
}

func TestAnnotationsDataSource(t *testing.T) {
	f := newFakeAPI(t)
	path := tenantPath("/monitoring", "/annotations")
	dur := int64(60000)
	f.on("GET "+path, ok([]any{map[string]any{"id": "a1", "label": "Deploy v2", "occurredAt": "2026-10-02T10:00:00Z",
		"durationMs": dur, "source": "deploy", "refKind": "service", "refId": "checkout", "tags": "release,canary"}}))

	got := tfDataRead(t, f, NewAnnotationsDataSource(), annotationsDataSourceModel{
		From: types.StringValue("2026-10-01T00:00:00Z"), RefKind: types.StringValue("service"),
		RefID: types.StringValue("checkout"), IncludeUnscoped: types.BoolValue(true), Limit: types.Int64Value(50),
	})
	q := f.last("GET " + path).Query
	for _, want := range []string{"from=2026-10-01", "refKind=service", "refId=checkout", "includeUnscoped=true", "limit=50"} {
		mustContain(t, q, want)
	}
	if len(got.Annotations) != 1 {
		t.Fatalf("annotations = %+v", got.Annotations)
	}
	a := got.Annotations[0]
	if a.DurationMs.ValueInt64() != 60000 || a.Source.ValueString() != "deploy" || len(a.Tags.Elements()) != 2 {
		t.Fatalf("annotation = %+v", a)
	}
	if !a.Description.IsNull() {
		t.Fatal("absent description must be null")
	}
}

// A re-enrolled operator leaves a Disabled monitor under the same cluster name;
// the lookup must return the live one whatever the list order.
func TestKubernetesClusterDataSourcePrefersLiveMonitor(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET "+tenantPath("/monitoring", "/monitors/kubernetes"), ok([]any{
		map[string]any{"id": "old", "name": "Kubernetes: prod", "clusterName": "prod", "agentId": "a0", "status": "Disabled"},
		map[string]any{"id": "other", "name": "Kubernetes: dev", "clusterName": "dev", "agentId": "a2", "status": "Active"},
		map[string]any{"id": "live", "name": "Kubernetes: prod", "clusterName": "prod", "agentId": "a1", "status": "Active",
			"slaProfileId": "sla1", "tags": "env:prod"},
	}))
	got := tfDataRead(t, f, NewKubernetesClusterDataSource(), kubernetesClusterModel{ClusterName: types.StringValue("prod"), Tags: types.SetNull(types.StringType)})
	if got.ID.ValueString() != "live" || got.AgentID.ValueString() != "a1" || got.SlaProfileID.ValueString() != "sla1" {
		t.Fatalf("state = %+v", got)
	}
	if len(got.Tags.Elements()) != 1 {
		t.Fatalf("tags = %v", got.Tags)
	}
}
