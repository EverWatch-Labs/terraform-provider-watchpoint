package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// derived_metric_rule maps onto the Ingestion API's DerivedMetricRule: a rule
// that emits a metric series into metric_rollups_hourly on a schedule, so a
// derived series is readable everywhere an ingested metric is (dashboards, the
// metrics explorer, alert rules).
//
// ONE resource, two sources, because the platform models it as one entity:
//   - source = "span"   counts matched spans, or measures their duration or a
//     numeric attribute, broken down by the promoted `labels`.
//   - source = "metric" a recording rule: folds an existing series to one
//     precomputed value per hour with `aggregation`.
//
// The emitted name is NOT the configured name. The API lower-cases it, strips
// anything outside [a-z0-9.] and forces the reserved prefix for the source
// ("span." or "rule."), so `metric_name` is Computed and carries what to
// reference from a dashboard or an alert. `name` keeps the configured spelling,
// for the same reason cost_allocation_tag keeps its tag key: writing a
// server-normalised value back into a Required attribute fails the apply.
//
// EVERYTHING EXCEPT `enabled` FORCES REPLACEMENT. The API's only update is
// PATCH {enabled}, so a rule's shape genuinely cannot be edited in place; the
// resource says so rather than silently no-opping a change. That is also why
// applyResponse keeps the configured values (see keepOrTakeString): those
// attributes cannot drift, and taking the server's copy would instead break
// every apply the API normalises - a trimmed service name, a lower-cased
// aggregation, or a recording rule coercing `value` back to `count`.
//
// There is no GET-by-id on the platform, so Read lists the tenant's rules (the
// listing is capped at 10 per tenant by DerivedMetricRulePolicy) and picks ours
// out, exactly like cost_allocation_tag.
//
// `labels` is a Set, never a List: it is unordered membership, and the API
// normalises and de-duplicates the keys.
//
// Requires the `derived_metric_rules` feature flag on the tenant. These routes
// live under /api/v1/observability/, which the API-key scope map does not
// cover, so writes fall through to the default-deny and need the broad
// `settings.manage` scope on the key.

var (
	_ resource.Resource                = (*derivedMetricRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*derivedMetricRuleResource)(nil)
	_ resource.ResourceWithImportState = (*derivedMetricRuleResource)(nil)
)

// derivedMetricRuleSources are the two rule kinds, wire form (lower-case).
var derivedMetricRuleSources = []string{"span", "metric"}

// derivedMetricRuleAggregations mirrors DerivedMetricRulePolicy.Aggregations.
// The evaluator maps the key onto a fixed SQL expression, so anything else is
// a 400.
var derivedMetricRuleAggregations = []string{"sum", "avg", "min", "max", "count", "p50", "p95", "p99"}

// derivedMetricRuleFlatLabels are label keys served from a flat span column
// rather than the attribute bag; every other key is read out of the span's
// attributes and is screened against the tenant's cardinality estimate.
var derivedMetricRuleFlatLabels = []string{"service", "operation", "status"}

// Ceilings from DerivedMetricRulePolicy, mirrored so a plan fails locally
// instead of at apply time.
const (
	derivedMetricRuleMaxLabels  = 3
	derivedMetricRuleMaxFilters = 5
)

// NewDerivedMetricRuleResource is the resource constructor wired into the provider.
func NewDerivedMetricRuleResource() resource.Resource { return &derivedMetricRuleResource{} }

type derivedMetricRuleResource struct {
	client *client.Client
}

// derivedMetricRuleModel maps schema attributes <-> Terraform state.
type derivedMetricRuleModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	MetricName  types.String `tfsdk:"metric_name"`
	Source      types.String `tfsdk:"source"`
	Type        types.String `tfsdk:"type"`
	Service     types.String `tfsdk:"service"`
	Operation   types.String `tfsdk:"operation"`
	Labels      types.Set    `tfsdk:"labels"`
	Filters     types.Map    `tfsdk:"filters"`
	Value       types.String `tfsdk:"value"`
	Metric      types.String `tfsdk:"metric"`
	Aggregation types.String `tfsdk:"aggregation"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	SeriesCount types.Int64  `tfsdk:"series_count"`
	LastRunAt   types.String `tfsdk:"last_run_at"`
	LastStatus  types.String `tfsdk:"last_status"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the Ingestion API) ---

// derivedMetricRuleCreateBody mirrors CreateDerivedMetricRuleRequest.
type derivedMetricRuleCreateBody struct {
	Name        string            `json:"name"`
	Source      string            `json:"source"`
	Labels      []string          `json:"labels,omitempty"`
	Service     *string           `json:"service,omitempty"`
	Operation   *string           `json:"operation,omitempty"`
	Filters     map[string]string `json:"filters,omitempty"`
	Value       *string           `json:"value,omitempty"`
	Metric      *string           `json:"metric,omitempty"`
	Aggregation *string           `json:"aggregation,omitempty"`
}

// derivedMetricRuleUpdateBody mirrors UpdateDerivedMetricRuleRequest. The API
// rejects an absent flag rather than binding it to false, so this is never
// `omitempty`.
type derivedMetricRuleUpdateBody struct {
	Enabled *bool `json:"enabled"`
}

// derivedMetricRuleResponse mirrors DerivedMetricRuleDto.
type derivedMetricRuleResponse struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Source      string            `json:"source"`
	Type        string            `json:"type"`
	Labels      []string          `json:"labels"`
	Service     *string           `json:"service"`
	Operation   *string           `json:"operation"`
	Filters     map[string]string `json:"filters"`
	Value       *string           `json:"value"`
	Metric      *string           `json:"metric"`
	Aggregation *string           `json:"aggregation"`
	Enabled     bool              `json:"enabled"`
	LastRunAt   *string           `json:"lastRunAt"`
	LastStatus  *string           `json:"lastStatus"`
	CreatedAt   string            `json:"createdAt"`
	SeriesCount int64             `json:"seriesCount"`
}

// derivedMetricRuleListResponse mirrors DerivedMetricRuleListResponse.
type derivedMetricRuleListResponse struct {
	Items []derivedMetricRuleResponse `json:"items"`
}

func (r *derivedMetricRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_derived_metric_rule"
}

func (r *derivedMetricRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

// rulesPath builds an /observability/otlp/derived-rules path. These routes carry
// the tenant id at the END of the prefix instead of the "/tenants/<id>/" position
// every other service uses, so client.Tenantf cannot build them.
func (r *derivedMetricRuleResource) rulesPath(format string, args ...any) string {
	return fmt.Sprintf("/observability/otlp/derived-rules/%s%s",
		r.client.TenantID(), fmt.Sprintf(format, args...))
}

func (r *derivedMetricRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A derived metric rule: a span-derived metric (`source = \"span\"`) or a " +
			"recording rule (`source = \"metric\"`) that emits an hourly series into the tenant's metric " +
			"rollups, where dashboards, the metrics explorer and alert rules read it like any ingested " +
			"metric. The emitted series is named by `metric_name`, not by `name`: the API forces it into " +
			"the reserved `span.` or `rule.` namespace so it can never collide with an ingested metric. " +
			"Only `enabled` can be changed in place; every other attribute replaces the rule. Requires " +
			"the `derived_metric_rules` feature flag, and an API key with the `settings.manage` scope.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Derived metric rule ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name to derive the emitted metric from. The API lower-cases it, " +
					"replaces separators with `.` and prefixes it with `span.` or `rule.`; the result is " +
					"`metric_name`. Write the fully qualified form (`span.checkout.errors`) to have the " +
					"two match exactly. Changing it replaces the rule.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 128)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"metric_name": schema.StringAttribute{
				MarkdownDescription: "The emitted metric name, in the reserved namespace for the source. " +
					"This is what a dashboard widget or alert rule queries.",
				Computed: true,
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "What the rule reads: `" + strings.Join(derivedMetricRuleSources, "`, `") +
					"`. Defaults to `span`. Changing it replaces the rule.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.OneOf(derivedMetricRuleSources...)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "The metric type the rule emits, derived by the API from the rule " +
					"itself: `counter` for a span count, `histogram` for a measured span value, `gauge` " +
					"for a recording rule.",
				Computed: true,
			},
			"service": schema.StringAttribute{
				MarkdownDescription: "Span rules only, and REQUIRED for them: the single `service.name` to " +
					"match. There is deliberately no all-services rule, because the service is what keeps " +
					"the hourly scan an index range instead of the whole span firehose. Changing it " +
					"replaces the rule.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.LengthBetween(1, 256)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"operation": schema.StringAttribute{
				MarkdownDescription: "Span rules only: an optional span name (operation) filter. Omit to " +
					"match every operation in the service. Changing it replaces the rule.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"labels": schema.SetAttribute{
				MarkdownDescription: "Span rules only: up to " + fmt.Sprint(derivedMetricRuleMaxLabels) +
					" label keys to break the metric down by. `" +
					strings.Join(derivedMetricRuleFlatLabels, "`, `") + "` come from the span's own " +
					"columns; any other key is read from the span attributes and is refused when that " +
					"key's distinct-value estimate is over the tenant's cardinality limit, because " +
					"promoting it would create one series per value. Changing the set replaces the rule.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Validators:  []validator.Set{setvalidator.SizeAtMost(derivedMetricRuleMaxLabels)},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
					setplanmodifier.RequiresReplace(),
				},
			},
			"filters": schema.MapAttribute{
				MarkdownDescription: "Span rules only: up to " + fmt.Sprint(derivedMetricRuleMaxFilters) +
					" span-attribute equality predicates, for example `{ \"http.request.method\" = " +
					"\"POST\" }`. Changing them replaces the rule.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Validators:  []validator.Map{mapvalidator.SizeAtMost(derivedMetricRuleMaxFilters)},
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
					mapplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "Span rules only: what is measured per matched span. `count` (one " +
					"unit per span, the default), `duration` (the span duration in milliseconds), or " +
					"`attr:<key>` for a numeric span attribute - a non-numeric value is skipped, not " +
					"counted as zero. Ignored for a recording rule. Changing it replaces the rule.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"metric": schema.StringAttribute{
				MarkdownDescription: "Recording rules only, and REQUIRED for them: the existing metric the " +
					"rule reads. It may not itself be a recording rule (anything in the `rule.` " +
					"namespace), which is what makes evaluation cycles impossible. Changing it replaces " +
					"the rule.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.LengthBetween(1, 256)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"aggregation": schema.StringAttribute{
				MarkdownDescription: "Recording rules only, and REQUIRED for them: how the source series' " +
					"hour is folded to one value. One of `" +
					strings.Join(derivedMetricRuleAggregations, "`, `") + "`. Changing it replaces the rule.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.OneOf(derivedMetricRuleAggregations...)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the rule is evaluated. Defaults to `true`. Pausing a rule " +
					"stops it at the next cycle and keeps the series it has already emitted; deleting the " +
					"rule drops those buckets too. Resuming does NOT back-fill the paused hours beyond " +
					"the evaluator's own floor.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"series_count": schema.Int64Attribute{
				MarkdownDescription: "How many distinct label series the rule has emitted. Worth watching: " +
					"each one is a billable metric series.",
				Computed: true,
			},
			"last_run_at": schema.StringAttribute{
				MarkdownDescription: "When the evaluator last processed this rule (RFC 3339), if ever.",
				Computed:            true,
			},
			"last_status": schema.StringAttribute{
				MarkdownDescription: "Last failure reason (over the scan budget, an invalid filter, ...), " +
					"or null when the last cycle succeeded.",
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *derivedMetricRuleModel) toCreateBody(ctx context.Context, diags *diagSink) *derivedMetricRuleCreateBody {
	body := &derivedMetricRuleCreateBody{
		Name:        m.Name.ValueString(),
		Source:      m.Source.ValueString(), // "" for an unset Optional+Computed; the API defaults it to span
		Labels:      stringSliceFromSet(ctx, m.Labels, diags.d),
		Service:     strPtr(m.Service),
		Operation:   strPtr(m.Operation),
		Value:       strPtr(m.Value),
		Metric:      strPtr(m.Metric),
		Aggregation: strPtr(m.Aggregation),
	}
	if !m.Filters.IsNull() && !m.Filters.IsUnknown() {
		filters := map[string]string{}
		diags.d.Append(m.Filters.ElementsAs(ctx, &filters, false)...)
		if len(filters) > 0 {
			body.Filters = filters
		}
	}
	return body
}

// keepOrTakeString returns the state value when there already is one, and the
// server's otherwise.
//
// Every attribute this guards forces replacement, so it cannot drift: the API's
// only update is PATCH {enabled}. Taking the server's copy would therefore never
// detect a real change, but WOULD break the apply on every value the API
// normalises - it trims a service name, lower-cases an aggregation, rewrites the
// name into a reserved namespace, and coerces `value` to `count` on a recording
// rule. The one case the server value is wanted is an import, where state has
// nothing yet. Same lesson as costAllocationTagModel.applyRow.
func keepOrTakeString(state types.String, server *string) types.String {
	if !state.IsNull() && !state.IsUnknown() {
		return state
	}
	return strFromPtr(server)
}

func (m *derivedMetricRuleModel) applyResponse(ctx context.Context, resp *derivedMetricRuleResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.MetricName = types.StringValue(resp.Name)
	m.Type = types.StringValue(resp.Type)

	// enabled is the ONE mutable attribute, so it is the one that takes the
	// server's value unconditionally: a rule paused in the portal is real drift
	// the next plan should propose putting back.
	m.Enabled = types.BoolValue(resp.Enabled)

	m.Name = keepOrTakeString(m.Name, &resp.Name)
	m.Source = keepOrTakeString(m.Source, &resp.Source)
	m.Service = keepOrTakeString(m.Service, resp.Service)
	m.Operation = keepOrTakeString(m.Operation, resp.Operation)
	m.Value = keepOrTakeString(m.Value, resp.Value)
	m.Metric = keepOrTakeString(m.Metric, resp.Metric)
	m.Aggregation = keepOrTakeString(m.Aggregation, resp.Aggregation)

	if m.Labels.IsNull() || m.Labels.IsUnknown() {
		m.Labels = setFromStringSlice(ctx, resp.Labels, diags.d)
	}
	if m.Filters.IsNull() || m.Filters.IsUnknown() {
		if len(resp.Filters) == 0 {
			m.Filters = types.MapNull(types.StringType)
		} else {
			v, d := types.MapValueFrom(ctx, types.StringType, resp.Filters)
			diags.d.Append(d...)
			m.Filters = v
		}
	}

	m.SeriesCount = types.Int64Value(resp.SeriesCount)
	m.LastRunAt = strFromPtr(resp.LastRunAt)
	m.LastStatus = strFromPtr(resp.LastStatus)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
}

// findDerivedMetricRule picks one rule out of a listing. There is no GET-by-id
// on the platform, so this is how Read addresses the row.
func findDerivedMetricRule(items []derivedMetricRuleResponse, id string) *derivedMetricRuleResponse {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func (r *derivedMetricRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan derivedMetricRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sink := &diagSink{d: &resp.Diagnostics}
	body := plan.toCreateBody(ctx, sink)
	if resp.Diagnostics.HasError() {
		return
	}

	var created derivedMetricRuleResponse
	if err := r.client.Post(ctx, r.rulesPath(""), body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating derived metric rule", err.Error())
		return
	}

	// Create always enables the rule; a plan that asks for a paused rule needs
	// the follow-up PATCH.
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() && !plan.Enabled.ValueBool() {
		var paused derivedMetricRuleResponse
		enabled := false
		if err := r.client.Patch(ctx, r.rulesPath("/%s", created.ID),
			&derivedMetricRuleUpdateBody{Enabled: &enabled}, &paused); err != nil {
			resp.Diagnostics.AddError("Error creating derived metric rule",
				"The rule was created but could not be paused: "+err.Error())
			return
		}
		created = paused
	}

	plan.applyResponse(ctx, &created, sink)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *derivedMetricRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state derivedMetricRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list derivedMetricRuleListResponse
	if err := r.client.Get(ctx, r.rulesPath(""), nil, &list); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading derived metric rule", err.Error())
		return
	}

	got := findDerivedMetricRule(list.Items, state.ID.ValueString())
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.applyResponse(ctx, got, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only ever runs for an `enabled` change; every other attribute forces
// replacement, because PATCH {enabled} is the platform's whole update surface.
func (r *derivedMetricRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan derivedMetricRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	enabled := plan.Enabled.ValueBool()
	var updated derivedMetricRuleResponse
	if err := r.client.Patch(ctx, r.rulesPath("/%s", plan.ID.ValueString()),
		&derivedMetricRuleUpdateBody{Enabled: &enabled}, &updated); err != nil {
		resp.Diagnostics.AddError("Error updating derived metric rule", err.Error())
		return
	}

	plan.applyResponse(ctx, &updated, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete also drops every hourly bucket the rule emitted, server-side, so the
// series disappears from the explorer rather than freezing until retention
// catches up.
func (r *derivedMetricRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state derivedMetricRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.Delete(ctx, r.rulesPath("/%s", state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting derived metric rule", err.Error())
	}
}

func (r *derivedMetricRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
