package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// SLA profiles define uptime/latency/error-rate targets over a rolling window
// and drive the SLA evaluator + breach ledger. This resource manages the SLA
// PROFILE only (CRUD); evaluation/breach/coverage endpoints are out of scope.
// Feature-gated server-side on the `sla_monitoring` flag (Business tier).

var (
	_ resource.Resource                   = (*slaProfileResource)(nil)
	_ resource.ResourceWithConfigure      = (*slaProfileResource)(nil)
	_ resource.ResourceWithImportState    = (*slaProfileResource)(nil)
	_ resource.ResourceWithValidateConfig = (*slaProfileResource)(nil)
)

// NewSLAProfileResource is the resource constructor wired into the provider.
func NewSLAProfileResource() resource.Resource { return &slaProfileResource{} }

type slaProfileResource struct {
	client *client.Client
}

// slaProfileModel maps schema attributes <-> Terraform state.
type slaProfileModel struct {
	ID                        types.String  `tfsdk:"id"`
	Name                      types.String  `tfsdk:"name"`
	Description               types.String  `tfsdk:"description"`
	TargetUptimePercent       types.Float64 `tfsdk:"target_uptime_percent"`
	WindowDays                types.Int64   `tfsdk:"window_days"`
	TargetP95LatencyMs        types.Int64   `tfsdk:"target_p95_latency_ms"`
	MaxErrorRatePercent       types.Float64 `tfsdk:"max_error_rate_percent"`
	AtRiskThresholdPercent    types.Float64 `tfsdk:"at_risk_threshold_percent"`
	ExcludeMaintenanceWindows types.Bool    `tfsdk:"exclude_maintenance_windows"`
	AlertOnBreach             types.Bool    `tfsdk:"alert_on_breach"`
	AlertOnAtRisk             types.Bool    `tfsdk:"alert_on_at_risk"`
	AlertProfileID            types.String  `tfsdk:"alert_profile_id"`
	IsDefault                 types.Bool    `tfsdk:"is_default"`
	SourceKind                types.String  `tfsdk:"source_kind"`
	SourceConfigJSON          types.String  `tfsdk:"source_config_json"`
	AttachedCount             types.Int64   `tfsdk:"attached_count"`
	CreatedAt                 types.String  `tfsdk:"created_at"`
	UpdatedAt                 types.String  `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the SLA API) ---

type slaProfileCreateBody struct {
	Name                      string   `json:"name"`
	Description               *string  `json:"description,omitempty"`
	TargetUptimePercent       float64  `json:"targetUptimePercent"`
	WindowDays                int64    `json:"windowDays"`
	TargetP95LatencyMs        *int64   `json:"targetP95LatencyMs,omitempty"`
	MaxErrorRatePercent       *float64 `json:"maxErrorRatePercent,omitempty"`
	AtRiskThresholdPercent    *float64 `json:"atRiskThresholdPercent,omitempty"`
	ExcludeMaintenanceWindows *bool    `json:"excludeMaintenanceWindows,omitempty"`
	AlertOnBreach             *bool    `json:"alertOnBreach,omitempty"`
	AlertOnAtRisk             *bool    `json:"alertOnAtRisk,omitempty"`
	AlertProfileID            *string  `json:"alertProfileId,omitempty"`
	IsDefault                 *bool    `json:"isDefault,omitempty"`
	SourceKind                *string  `json:"sourceKind,omitempty"`
	SourceConfigJSON          *string  `json:"sourceConfigJson,omitempty"`
}

// slaProfileUpdateBody mirrors the PATCH DTO (all fields optional). WindowDays
// and TargetUptimePercent are pointers here so an unchanged value can be
// omitted; the create path sends them as required scalars.
type slaProfileUpdateBody struct {
	Name                      *string  `json:"name,omitempty"`
	Description               *string  `json:"description,omitempty"`
	TargetUptimePercent       *float64 `json:"targetUptimePercent,omitempty"`
	WindowDays                *int64   `json:"windowDays,omitempty"`
	TargetP95LatencyMs        *int64   `json:"targetP95LatencyMs,omitempty"`
	MaxErrorRatePercent       *float64 `json:"maxErrorRatePercent,omitempty"`
	AtRiskThresholdPercent    *float64 `json:"atRiskThresholdPercent,omitempty"`
	ExcludeMaintenanceWindows *bool    `json:"excludeMaintenanceWindows,omitempty"`
	AlertOnBreach             *bool    `json:"alertOnBreach,omitempty"`
	AlertOnAtRisk             *bool    `json:"alertOnAtRisk,omitempty"`
	AlertProfileID            *string  `json:"alertProfileId,omitempty"`
	IsDefault                 *bool    `json:"isDefault,omitempty"`
	SourceKind                *string  `json:"sourceKind,omitempty"`
	// "" clears the config; omitted would keep a removed one.
	SourceConfigJSON *string `json:"sourceConfigJson,omitempty"`
}

type slaProfileResponse struct {
	ID                        string   `json:"id"`
	Name                      string   `json:"name"`
	Description               *string  `json:"description"`
	TargetUptimePercent       float64  `json:"targetUptimePercent"`
	WindowDays                int64    `json:"windowDays"`
	TargetP95LatencyMs        *int64   `json:"targetP95LatencyMs"`
	MaxErrorRatePercent       *float64 `json:"maxErrorRatePercent"`
	AtRiskThresholdPercent    float64  `json:"atRiskThresholdPercent"`
	ExcludeMaintenanceWindows bool     `json:"excludeMaintenanceWindows"`
	AlertOnBreach             bool     `json:"alertOnBreach"`
	AlertOnAtRisk             bool     `json:"alertOnAtRisk"`
	AlertProfileID            *string  `json:"alertProfileId"`
	IsDefault                 bool     `json:"isDefault"`
	SourceKind                string   `json:"sourceKind"`
	SourceConfigJSON          *string  `json:"sourceConfigJson"`
	AttachedCount             int64    `json:"attachedCount"`
	CreatedAt                 string   `json:"createdAt"`
	UpdatedAt                 string   `json:"updatedAt"`
}

func (r *slaProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sla_profile"
}

func (r *slaProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *slaProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An SLA profile defining uptime/latency/error-rate targets over a rolling " +
			"window. Requires the `sla_monitoring` feature (Business tier).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "SLA profile ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable profile name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Optional:            true,
			},
			"target_uptime_percent": schema.Float64Attribute{
				MarkdownDescription: "Target uptime percentage in the range (0, 100].",
				Required:            true,
			},
			"window_days": schema.Int64Attribute{
				MarkdownDescription: "Rolling evaluation window in days (1-365).",
				Required:            true,
			},
			"target_p95_latency_ms": schema.Int64Attribute{
				MarkdownDescription: "Optional p95 latency target in milliseconds.",
				Optional:            true,
			},
			"max_error_rate_percent": schema.Float64Attribute{
				MarkdownDescription: "Optional maximum allowed error rate percentage.",
				Optional:            true,
			},
			"at_risk_threshold_percent": schema.Float64Attribute{
				MarkdownDescription: "Error-budget consumption (percent) at which the SLA is flagged at-risk. Defaults to `75`.",
				Optional:            true,
				Computed:            true,
				Default:             float64default.StaticFloat64(75.0),
			},
			"exclude_maintenance_windows": schema.BoolAttribute{
				MarkdownDescription: "Exclude scheduled maintenance windows from SLA calculations. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"alert_on_breach": schema.BoolAttribute{
				MarkdownDescription: "Raise an alert when the SLA is breached. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"alert_on_at_risk": schema.BoolAttribute{
				MarkdownDescription: "Raise an alert when the SLA becomes at-risk. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"alert_profile_id": schema.StringAttribute{
				MarkdownDescription: "Alert profile used for SLA notifications.",
				Optional:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the tenant's default SLA profile. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"source_kind": schema.StringAttribute{
				MarkdownDescription: "Where attainment comes from. `Monitor` (default) scores the monitors and " +
					"groups attached to the profile. `Traces` scores an OTLP service or operation from its spans " +
					"(error rate, and latency against an objective); `Metrics` scores the ratio of two OTLP " +
					"metrics, good over total. Telemetry sources are not attached to monitors and get the same " +
					"multi-window burn-rate alerts as monitor SLOs. Both require the `slo_trace_source` feature.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString("Monitor"),
				Validators: []validator.String{stringvalidator.OneOf("Monitor", "Traces", "Metrics")},
			},
			"source_config_json": schema.StringAttribute{
				MarkdownDescription: "JSON configuration for a telemetry source; not allowed with `Monitor`. `Traces`: `{\"serviceName\":\"checkout\",\"operationName\":\"POST /pay\"," +
					"\"objectiveMs\":300}` (operation and objective optional). `Metrics`: " +
					"`{\"goodMetric\":\"http.requests.ok\",\"totalMetric\":\"http.requests\"}`, both required. " +
					"Use `jsonencode(...)`.",
				Optional: true,
			},
			"attached_count": schema.Int64Attribute{
				MarkdownDescription: "Number of monitors and groups attached to this profile.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "Last-update timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

// ValidateConfig rejects source_config_json on a Monitor-sourced profile. The
// API discards it for Monitor, so it would read back null against the planned
// value and fail the apply as an inconsistent result.
func (r *slaProfileResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var kind, cfg types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("source_kind"), &kind)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("source_config_json"), &cfg)...)
	if cfg.IsNull() || kind.IsUnknown() {
		return
	}
	// Unset source_kind defaults to Monitor.
	if kind.IsNull() || kind.ValueString() == "Monitor" {
		resp.Diagnostics.AddAttributeError(path.Root("source_config_json"), "source_config_json requires a telemetry source",
			"source_config_json only applies when source_kind is \"Traces\" or \"Metrics\"; remove it or set source_kind.")
	}
}

func (m *slaProfileModel) toCreateBody() *slaProfileCreateBody {
	return &slaProfileCreateBody{
		Name:                      m.Name.ValueString(),
		Description:               strPtr(m.Description),
		TargetUptimePercent:       m.TargetUptimePercent.ValueFloat64(),
		WindowDays:                m.WindowDays.ValueInt64(),
		TargetP95LatencyMs:        int64Ptr(m.TargetP95LatencyMs),
		MaxErrorRatePercent:       float64Ptr(m.MaxErrorRatePercent),
		AtRiskThresholdPercent:    float64Ptr(m.AtRiskThresholdPercent),
		ExcludeMaintenanceWindows: boolPtr(m.ExcludeMaintenanceWindows),
		AlertOnBreach:             boolPtr(m.AlertOnBreach),
		AlertOnAtRisk:             boolPtr(m.AlertOnAtRisk),
		AlertProfileID:            strPtr(m.AlertProfileID),
		IsDefault:                 boolPtr(m.IsDefault),
		SourceKind:                strPtr(m.SourceKind),
		SourceConfigJSON:          strPtr(m.SourceConfigJSON),
	}
}

func (m *slaProfileModel) toUpdateBody() *slaProfileUpdateBody {
	return &slaProfileUpdateBody{
		Name:                      strPtr(m.Name),
		Description:               strPtr(m.Description),
		TargetUptimePercent:       float64Ptr(m.TargetUptimePercent),
		WindowDays:                int64Ptr(m.WindowDays),
		TargetP95LatencyMs:        int64Ptr(m.TargetP95LatencyMs),
		MaxErrorRatePercent:       float64Ptr(m.MaxErrorRatePercent),
		AtRiskThresholdPercent:    float64Ptr(m.AtRiskThresholdPercent),
		ExcludeMaintenanceWindows: boolPtr(m.ExcludeMaintenanceWindows),
		AlertOnBreach:             boolPtr(m.AlertOnBreach),
		AlertOnAtRisk:             boolPtr(m.AlertOnAtRisk),
		AlertProfileID:            strPtr(m.AlertProfileID),
		IsDefault:                 boolPtr(m.IsDefault),
		SourceKind:                strPtr(m.SourceKind),
		SourceConfigJSON:          strPtrClearing(m.SourceConfigJSON),
	}
}

// applyResponse copies API response fields into the model.
func (m *slaProfileModel) applyResponse(resp *slaProfileResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.TargetUptimePercent = types.Float64Value(resp.TargetUptimePercent)
	m.WindowDays = types.Int64Value(resp.WindowDays)
	m.TargetP95LatencyMs = int64FromPtr(resp.TargetP95LatencyMs)
	m.MaxErrorRatePercent = float64FromPtr(resp.MaxErrorRatePercent)
	m.AtRiskThresholdPercent = types.Float64Value(resp.AtRiskThresholdPercent)
	m.ExcludeMaintenanceWindows = types.BoolValue(resp.ExcludeMaintenanceWindows)
	m.AlertOnBreach = types.BoolValue(resp.AlertOnBreach)
	m.AlertOnAtRisk = types.BoolValue(resp.AlertOnAtRisk)
	m.AlertProfileID = strFromPtr(resp.AlertProfileID)
	m.IsDefault = types.BoolValue(resp.IsDefault)
	m.SourceKind = types.StringValue(resp.SourceKind)
	var cfg []byte
	if resp.SourceConfigJSON != nil {
		cfg = []byte(*resp.SourceConfigJSON)
	}
	m.SourceConfigJSON = jsonKeepingConfig(m.SourceConfigJSON, cfg)
	m.AttachedCount = types.Int64Value(resp.AttachedCount)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
}

func (r *slaProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan slaProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toCreateBody()

	// POST returns the full mapped profile object.
	var created slaProfileResponse
	path := r.client.Tenantf("/monitoring", "/sla-profiles")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating SLA profile", err.Error())
		return
	}

	plan.applyResponse(&created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *slaProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state slaProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got slaProfileResponse
	getPath := r.client.Tenantf("/monitoring", "/sla-profiles/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading SLA profile", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *slaProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan slaProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toUpdateBody()

	// PATCH returns 204 No Content, so re-GET to refresh state.
	path := r.client.Tenantf("/monitoring", "/sla-profiles/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating SLA profile", err.Error())
		return
	}

	var got slaProfileResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading SLA profile after update", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *slaProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state slaProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/sla-profiles/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting SLA profile", err.Error())
	}
}

func (r *slaProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
