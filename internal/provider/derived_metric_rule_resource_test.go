package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func strp(s string) *string { return &s }

// The API rewrites the name into a reserved namespace ("checkout errors" ->
// "span.checkout.errors"). Writing that back into the Required `name` attribute
// fails the apply with "was cty.StringVal(\"checkout errors\"), but now
// cty.StringVal(\"span.checkout.errors\")", so the configured spelling has to
// survive and the emitted name has to land in metric_name instead.
func TestDerivedRuleApplyResponseKeepsTheConfiguredName(t *testing.T) {
	var diags diag.Diagnostics
	m := derivedMetricRuleModel{
		Name:    types.StringValue("checkout errors"),
		Source:  types.StringValue("span"),
		Service: types.StringValue("checkout-api"),
		Labels:  types.SetNull(types.StringType),
		Filters: types.MapNull(types.StringType),
	}
	m.applyResponse(context.Background(), &derivedMetricRuleResponse{
		ID: "dmr_1", Name: "span.checkout.errors", Source: "span", Type: "counter",
		Service: strp("checkout-api"), Enabled: true, CreatedAt: "2026-09-17T00:00:00Z",
	}, &diagSink{d: &diags})

	if m.Name.ValueString() != "checkout errors" {
		t.Fatalf("name was rewritten to %q", m.Name.ValueString())
	}
	if m.MetricName.ValueString() != "span.checkout.errors" {
		t.Fatalf("metric_name did not take the emitted name: %q", m.MetricName.ValueString())
	}
	if m.ID.ValueString() != "dmr_1" || m.Type.ValueString() != "counter" {
		t.Fatalf("id/type not taken from the response: %q %q", m.ID.ValueString(), m.Type.ValueString())
	}
}

// A recording rule ignores `value` and stores "count". Taking the server's copy
// would replace a configured "duration" with "count" after apply, which the
// framework rejects as an inconsistent result. Same class as the name above, and
// safe because every one of these attributes forces replacement, so none of them
// can drift.
func TestDerivedRuleApplyResponseKeepsConfiguredImmutableFields(t *testing.T) {
	var diags diag.Diagnostics
	m := derivedMetricRuleModel{
		Name:        types.StringValue("rule.checkout.p95"),
		Source:      types.StringValue("metric"),
		Value:       types.StringValue("duration"),
		Metric:      types.StringValue("http.server.duration"),
		Aggregation: types.StringValue("p95"),
		Labels:      types.SetNull(types.StringType),
		Filters:     types.MapNull(types.StringType),
	}
	m.applyResponse(context.Background(), &derivedMetricRuleResponse{
		ID: "dmr_2", Name: "rule.checkout.p95", Source: "metric", Type: "gauge",
		Value: strp("count"), Metric: strp("http.server.duration"), Aggregation: strp("p95"),
		Enabled: true, CreatedAt: "2026-09-17T00:00:00Z",
	}, &diagSink{d: &diags})

	if m.Value.ValueString() != "duration" {
		t.Fatalf("value was rewritten to %q", m.Value.ValueString())
	}
	if m.Aggregation.ValueString() != "p95" {
		t.Fatalf("aggregation was rewritten to %q", m.Aggregation.ValueString())
	}
}

// `enabled` is the one attribute the API can change (PATCH {enabled}), so it is
// the one that MUST take the server's value: a rule paused in the portal is real
// drift the next plan should propose putting back.
func TestDerivedRuleApplyResponseTakesServerEnabled(t *testing.T) {
	var diags diag.Diagnostics
	m := derivedMetricRuleModel{
		Name: types.StringValue("span.x"), Enabled: types.BoolValue(true),
		Labels: types.SetNull(types.StringType), Filters: types.MapNull(types.StringType),
	}
	m.applyResponse(context.Background(), &derivedMetricRuleResponse{
		ID: "dmr_3", Name: "span.x", Source: "span", Type: "counter",
		Enabled: false, SeriesCount: 12, LastStatus: strp("hour skipped, over budget"),
		CreatedAt: "2026-09-17T00:00:00Z",
	}, &diagSink{d: &diags})

	if m.Enabled.ValueBool() {
		t.Fatal("enabled stayed true after the server reported a paused rule")
	}
	if m.SeriesCount.ValueInt64() != 12 {
		t.Fatalf("series_count not taken from the response: %d", m.SeriesCount.ValueInt64())
	}
	if m.LastStatus.ValueString() != "hour skipped, over budget" {
		t.Fatalf("last_status not taken from the response: %q", m.LastStatus.ValueString())
	}
}

// On import state holds nothing but the id, so the server value is exactly what
// is wanted. The keep-the-config rule must not leave every attribute null.
func TestDerivedRuleApplyResponseFillsEmptyStateOnImport(t *testing.T) {
	var diags diag.Diagnostics
	m := derivedMetricRuleModel{
		ID:      types.StringValue("dmr_4"),
		Labels:  types.SetNull(types.StringType),
		Filters: types.MapNull(types.StringType),
	}
	m.applyResponse(context.Background(), &derivedMetricRuleResponse{
		ID: "dmr_4", Name: "span.checkout.errors", Source: "span", Type: "counter",
		Labels: []string{"status", "http.route"}, Service: strp("checkout-api"),
		Filters: map[string]string{"http.request.method": "POST"},
		Value:   strp("count"), Enabled: true, CreatedAt: "2026-09-17T00:00:00Z",
	}, &diagSink{d: &diags})

	if m.Name.ValueString() != "span.checkout.errors" {
		t.Fatalf("name not filled on import: %q", m.Name.ValueString())
	}
	if m.Service.ValueString() != "checkout-api" {
		t.Fatalf("service not filled on import: %q", m.Service.ValueString())
	}
	if m.Labels.IsNull() || len(m.Labels.Elements()) != 2 {
		t.Fatalf("labels not filled on import: %v", m.Labels)
	}
	if m.Filters.IsNull() || len(m.Filters.Elements()) != 1 {
		t.Fatalf("filters not filled on import: %v", m.Filters)
	}
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
}

// There is no GET-by-id on the platform, so Read addresses the row by scanning
// the listing. Missing it would silently remove a live rule from state and then
// recreate it, which for this resource also deletes every bucket it has emitted.
func TestFindDerivedMetricRule(t *testing.T) {
	items := []derivedMetricRuleResponse{
		{ID: "dmr_1", Name: "span.a"},
		{ID: "dmr_2", Name: "rule.b"},
	}
	if got := findDerivedMetricRule(items, "dmr_2"); got == nil || got.Name != "rule.b" {
		t.Fatalf("expected rule.b, got %v", got)
	}
	if got := findDerivedMetricRule(items, "dmr_9"); got != nil {
		t.Fatalf("expected no match for a deleted rule, got %v", got)
	}
}

// Labels are unordered membership and the API normalises + de-duplicates them,
// so the create body sends whatever the Set holds. A List here would fail the
// apply with "Provider produced inconsistent result after apply" the moment the
// API returned them in another order.
func TestDerivedRuleCreateBody(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	labels, d := types.SetValueFrom(ctx, types.StringType, []string{"status", "http.route"})
	diags.Append(d...)
	filters, d := types.MapValueFrom(ctx, types.StringType, map[string]string{"http.request.method": "POST"})
	diags.Append(d...)

	m := derivedMetricRuleModel{
		Name:    types.StringValue("checkout errors"),
		Source:  types.StringValue("span"),
		Service: types.StringValue("checkout-api"),
		Labels:  labels,
		Filters: filters,
		Value:   types.StringValue("count"),
		Metric:  types.StringNull(),
	}
	body := m.toCreateBody(ctx, &diagSink{d: &diags})
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(body.Labels) != 2 {
		t.Fatalf("labels not sent: %v", body.Labels)
	}
	if body.Filters["http.request.method"] != "POST" {
		t.Fatalf("filters not sent: %v", body.Filters)
	}
	if body.Service == nil || *body.Service != "checkout-api" {
		t.Fatalf("service not sent: %v", body.Service)
	}
	// A span rule must not send a recording-rule field, or the API's own
	// validation would start reading it.
	if body.Metric != nil {
		t.Fatalf("metric should be omitted for a span rule, got %v", *body.Metric)
	}
}
