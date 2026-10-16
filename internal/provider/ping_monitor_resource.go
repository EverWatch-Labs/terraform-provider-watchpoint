package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// This file mirrors http_monitor_resource.go for the ICMP ping monitor: a typed
// model, wire DTOs, Schema, and Create/Read/Update/Delete that map the model onto
// the Watchpoint monitoring API ping DTOs.

var (
	_ resource.Resource                = (*pingMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*pingMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*pingMonitorResource)(nil)
)

// NewPingMonitorResource is the resource constructor wired into the provider.
func NewPingMonitorResource() resource.Resource { return &pingMonitorResource{} }

type pingMonitorResource struct {
	client *client.Client
}

// pingMonitorModel maps schema attributes <-> Terraform state.
type pingMonitorModel struct {
	ID                   types.String  `tfsdk:"id"`
	Name                 types.String  `tfsdk:"name"`
	Hostname             types.String  `tfsdk:"hostname"`
	PingCount            types.Int64   `tfsdk:"ping_count"`
	MaxLatencyMs         types.Int64   `tfsdk:"max_latency_ms"`
	MaxPacketLossPercent types.Float64 `tfsdk:"max_packet_loss_percent"`
	CheckIntervalSeconds types.Int64   `tfsdk:"check_interval_seconds"`
	TimeoutSeconds       types.Int64   `tfsdk:"timeout_seconds"`
	LocationProfileID    types.String  `tfsdk:"location_profile_id"`
	ThresholdProfileID   types.String  `tfsdk:"threshold_profile_id"`
	AlertProfileID       types.String  `tfsdk:"alert_profile_id"`
	SlaProfileID         types.String  `tfsdk:"sla_profile_id"`
	GroupID              types.String  `tfsdk:"group_id"`
	Tags                 types.Set     `tfsdk:"tags"`
	Status               types.String  `tfsdk:"status"`
	CreatedAt            types.String  `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type pingMonitorCreateBody struct {
	Name                 string   `json:"name"`
	Hostname             string   `json:"hostname"`
	PingCount            *int64   `json:"pingCount,omitempty"`
	MaxLatencyMs         *int64   `json:"maxLatencyMs,omitempty"`
	MaxPacketLossPercent *float64 `json:"maxPacketLossPercent,omitempty"`
	CheckIntervalSeconds *int64   `json:"checkIntervalSeconds,omitempty"`
	TimeoutSeconds       *int64   `json:"timeoutSeconds,omitempty"`
	LocationProfileID    *string  `json:"locationProfileId,omitempty"`
	ThresholdProfileID   *string  `json:"thresholdProfileId,omitempty"`
	AlertProfileID       *string  `json:"alertProfileId,omitempty"`
	GroupID              *string  `json:"groupId,omitempty"`
	Tags                 *string  `json:"tags,omitempty"`
}

// pingMonitorUpdateBody mirrors the PATCH DTO; pointers omit unchanged fields.
type pingMonitorUpdateBody = pingMonitorCreateBody

type pingMonitorResponse struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Status               string  `json:"status"`
	Hostname             string  `json:"hostname"`
	PingCount            int64   `json:"pingCount"`
	MaxLatencyMs         int64   `json:"maxLatencyMs"`
	MaxPacketLossPercent float64 `json:"maxPacketLossPercent"`
	CheckIntervalSeconds int64   `json:"checkIntervalSeconds"`
	TimeoutSeconds       int64   `json:"timeoutSeconds"`
	LocationProfileID    *string `json:"locationProfileId"`
	ThresholdProfileID   *string `json:"thresholdProfileId"`
	AlertProfileID       *string `json:"alertProfileId"`
	SlaProfileID         *string `json:"slaProfileId"`
	GroupID              *string `json:"groupId"`
	Tags                 *string `json:"tags"`
	CreatedAt            string  `json:"createdAt"`
}

func (r *pingMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ping_monitor"
}

func (r *pingMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *pingMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An ICMP ping monitor that measures latency and packet loss to a host.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Monitor ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable monitor name; shown across the UI and alerts.",
				Required:            true,
			},
			"hostname": schema.StringAttribute{
				MarkdownDescription: "Hostname or IP to ping.",
				Required:            true,
			},
			"ping_count": schema.Int64Attribute{
				MarkdownDescription: "Number of echo requests sent per check. Defaults to `4`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(4),
			},
			"max_latency_ms": schema.Int64Attribute{
				MarkdownDescription: "Maximum acceptable average latency in milliseconds. Defaults to `500`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(500),
			},
			"max_packet_loss_percent": schema.Float64Attribute{
				MarkdownDescription: "Maximum acceptable packet-loss percentage before the check fails. Defaults to `20`.",
				Optional:            true,
				Computed:            true,
				Default:             float64default.StaticFloat64(20),
			},
			"check_interval_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds between checks. Defaults to `300`. Plans cap the minimum.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(300),
			},
			"timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Per-check timeout in seconds. Defaults to `30`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(30),
			},
			"location_profile_id": schema.StringAttribute{
				MarkdownDescription: "Profile that controls which regions run the check.",
				Optional:            true,
			},
			"threshold_profile_id": schema.StringAttribute{
				MarkdownDescription: "Profile that controls latency thresholds and consecutive-failure logic.",
				Optional:            true,
			},
			"alert_profile_id": schema.StringAttribute{
				MarkdownDescription: "Profile that controls notification channels and routing.",
				Optional:            true,
			},
			"sla_profile_id": schema.StringAttribute{
				MarkdownDescription: "SLA profile attached to this monitor for uptime/latency SLO and error-budget tracking. Requires the SLA monitoring feature.",
				Optional:            true,
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "Monitor group this monitor belongs to.",
				Optional:            true,
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "Free-form tags surfaced in filters and SGL queries (`attributes.tags`). Unordered.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current monitor status (e.g. `Active`, `Paused`).",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *pingMonitorModel) toCreateBody(ctx context.Context, diags *diagSink) *pingMonitorCreateBody {
	return &pingMonitorCreateBody{
		Name:                 m.Name.ValueString(),
		Hostname:             m.Hostname.ValueString(),
		PingCount:            int64Ptr(m.PingCount),
		MaxLatencyMs:         int64Ptr(m.MaxLatencyMs),
		MaxPacketLossPercent: float64Ptr(m.MaxPacketLossPercent),
		CheckIntervalSeconds: int64Ptr(m.CheckIntervalSeconds),
		TimeoutSeconds:       int64Ptr(m.TimeoutSeconds),
		LocationProfileID:    strPtr(m.LocationProfileID),
		ThresholdProfileID:   strPtr(m.ThresholdProfileID),
		AlertProfileID:       strPtr(m.AlertProfileID),
		GroupID:              strPtr(m.GroupID),
		Tags:                 csvPtrFromSet(ctx, m.Tags, diags.d),
	}
}

// applyResponse copies API response fields into the model.
func (m *pingMonitorModel) applyResponse(ctx context.Context, resp *pingMonitorResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Hostname = types.StringValue(resp.Hostname)
	m.PingCount = types.Int64Value(resp.PingCount)
	m.MaxLatencyMs = types.Int64Value(resp.MaxLatencyMs)
	m.MaxPacketLossPercent = types.Float64Value(resp.MaxPacketLossPercent)
	m.CheckIntervalSeconds = types.Int64Value(resp.CheckIntervalSeconds)
	m.TimeoutSeconds = types.Int64Value(resp.TimeoutSeconds)
	m.LocationProfileID = strFromPtr(resp.LocationProfileID)
	m.ThresholdProfileID = strFromPtr(resp.ThresholdProfileID)
	m.AlertProfileID = strFromPtr(resp.AlertProfileID)
	m.SlaProfileID = strFromPtr(resp.SlaProfileID)
	m.GroupID = strFromPtr(resp.GroupID)
	m.Tags = setFromCSVPtr(ctx, resp.Tags, diags.d)
	m.Status = types.StringValue(resp.Status)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
}

func (r *pingMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pingMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toCreateBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var created idNameResponse
	path := r.client.Tenantf("/monitoring", "/monitors/ping")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating ping monitor", err.Error())
		return
	}

	// SLA profile is attached via its own endpoint, not the monitor body.
	if slaIsSet(plan.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, created.ID, plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error attaching SLA profile to ping monitor", err.Error())
			return
		}
	}

	// Re-read to populate all computed fields from the canonical record.
	var got pingMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/ping/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading ping monitor after create", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pingMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pingMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got pingMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/ping/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading ping monitor", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *pingMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pingMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toCreateBody(ctx, ds)
	// An emptied tag set has to reach the API as "" or the old tags survive the PATCH.
	body.Tags = csvPtrClearing(ctx, plan.Tags, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/ping/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating ping monitor", err.Error())
		return
	}

	// Sync SLA attachment if it changed (set, swapped, or cleared).
	var priorState pingMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
	if !resp.Diagnostics.HasError() && !plan.SlaProfileID.Equal(priorState.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, plan.ID.ValueString(), plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error updating SLA profile on ping monitor", err.Error())
			return
		}
	}

	var got pingMonitorResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading ping monitor after update", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *pingMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pingMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/ping/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting ping monitor", err.Error())
	}
}

func (r *pingMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
