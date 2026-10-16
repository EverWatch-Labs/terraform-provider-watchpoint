package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The tenant's log pipeline (Ingestion, /observability/otlp/log-pipeline/{tenant}):
// ordered processors applied at ingest plus PII redaction rule overrides. A
// per-tenant singleton replaced whole by PUT. Processors and rules are a small
// polymorphic schema the API validates (and compiles) itself, so they are taken
// as JSON rather than re-modelled field by field here.

var (
	_ resource.Resource                = (*logPipelineResource)(nil)
	_ resource.ResourceWithConfigure   = (*logPipelineResource)(nil)
	_ resource.ResourceWithImportState = (*logPipelineResource)(nil)
)

// NewLogPipelineResource is the resource constructor wired into the provider.
func NewLogPipelineResource() resource.Resource { return &logPipelineResource{} }

type logPipelineResource struct {
	client *client.Client
}

type logPipelineModel struct {
	ID             types.String `tfsdk:"id"`
	ProcessorsJSON types.String `tfsdk:"processors_json"`
	PiiRulesJSON   types.String `tfsdk:"pii_rules_json"`
}

type logPipelineBody struct {
	Processors json.RawMessage `json:"processors"`
	PiiRules   json.RawMessage `json:"piiRules"`
}

type logPipelineResponse struct {
	Processors json.RawMessage `json:"processors"`
	PiiRules   json.RawMessage `json:"piiRules"`
}

func (r *logPipelineResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_pipeline"
}

func (r *logPipelineResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *logPipelineResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The tenant's log pipeline: ordered processors applied to every log at ingest, " +
			"then the tenant's PII redaction rules. A per-tenant singleton: declare it at most once. Other " +
			"pods pick up a change within 60 seconds. Processors require the `log_pipelines` feature (SMB); " +
			"PII rules work on every plan. Destroying the resource clears both lists, which restores the " +
			"platform's default redaction.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tenant ID, which this setting is keyed on.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"processors_json": schema.StringAttribute{
				MarkdownDescription: "JSON array of processors, run in order (50 max). Each has a `type` " +
					"(`grok`, `regex`, `json`, `set`, `rename`, `remove`, `severity`, `drop`, `route`) plus " +
					"`name`, `enabled`, an optional `filter` (for example `service:checkout -severity:debug`) and " +
					"the fields its type needs, such as `source`, `pattern` or `key`. Use camelCase keys, as " +
					"`jsonencode` of an HCL object does. Compared as JSON; defaults the API fills in are not drift.",
				Optional: true,
			},
			"pii_rules_json": schema.StringAttribute{
				MarkdownDescription: "JSON array of PII rules (25 max), each `{name, pattern, replacement, " +
					"appliesToFields, isActive}`. A rule named `email`, `card` or `ip` replaces that built-in " +
					"and an inactive one switches it off; others are added after the built-ins. JWT, SSN, " +
					"bearer and authorization redaction always run and cannot be overridden.",
				Optional: true,
			},
		},
	}
}

func (r *logPipelineResource) path() string {
	return fmt.Sprintf("/observability/otlp/log-pipeline/%s", r.client.TenantID())
}

// emptyArrayIfNil sends [] for an absent list: PUT replaces the whole config.
func emptyArrayIfNil(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return json.RawMessage("[]")
	}
	return raw
}

func (m *logPipelineModel) toBody() *logPipelineBody {
	return &logPipelineBody{
		Processors: emptyArrayIfNil(rawJSONOrNil(m.ProcessorsJSON)),
		PiiRules:   emptyArrayIfNil(rawJSONOrNil(m.PiiRulesJSON)),
	}
}

func (m *logPipelineModel) applyResponse(tenantID string, in *logPipelineResponse) {
	m.ID = types.StringValue(tenantID)
	m.ProcessorsJSON = jsonKeepingConfig(m.ProcessorsJSON, in.Processors)
	m.PiiRulesJSON = jsonKeepingConfig(m.PiiRulesJSON, in.PiiRules)
}

func (r *logPipelineResource) put(ctx context.Context, plan *logPipelineModel, ds *diagSink, action string) {
	var got logPipelineResponse
	if err := r.client.Put(ctx, r.path(), plan.toBody(), &got); err != nil {
		ds.d.AddError("Error "+action+" log pipeline", err.Error())
		return
	}
	plan.applyResponse(r.client.TenantID(), &got)
}

func (r *logPipelineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan logPipelineModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.put(ctx, &plan, &diagSink{d: &resp.Diagnostics}, "creating"); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *logPipelineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state logPipelineModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got logPipelineResponse
	if err := r.client.Get(ctx, r.path(), nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading log pipeline", err.Error())
		return
	}
	state.applyResponse(r.client.TenantID(), &got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *logPipelineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan logPipelineModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.put(ctx, &plan, &diagSink{d: &resp.Diagnostics}, "updating"); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *logPipelineResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	empty := logPipelineModel{ProcessorsJSON: types.StringNull(), PiiRulesJSON: types.StringNull()}
	if err := r.client.Put(ctx, r.path(), empty.toBody(), nil); err != nil {
		resp.Diagnostics.AddError("Error clearing log pipeline", err.Error())
	}
}

func (r *logPipelineResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
