package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// This file mirrors http_monitor_resource.go for the TCP port monitor: a typed
// model, wire DTOs, Schema, and Create/Read/Update/Delete that map the model onto
// the Watchpoint monitoring API TCP DTOs.

var (
	_ resource.Resource                = (*tcpMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*tcpMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*tcpMonitorResource)(nil)
)

// NewTCPMonitorResource is the resource constructor wired into the provider.
func NewTCPMonitorResource() resource.Resource { return &tcpMonitorResource{} }

type tcpMonitorResource struct {
	client *client.Client
}

// tcpMonitorModel maps schema attributes <-> Terraform state.
type tcpMonitorModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Hostname             types.String `tfsdk:"hostname"`
	Port                 types.Int64  `tfsdk:"port"`
	SendData             types.String `tfsdk:"send_data"`
	ExpectResponse       types.String `tfsdk:"expect_response"`
	UseTLS               types.Bool   `tfsdk:"use_tls"`
	CheckIntervalSeconds types.Int64  `tfsdk:"check_interval_seconds"`
	TimeoutSeconds       types.Int64  `tfsdk:"timeout_seconds"`
	LocationProfileID    types.String `tfsdk:"location_profile_id"`
	ThresholdProfileID   types.String `tfsdk:"threshold_profile_id"`
	AlertProfileID       types.String `tfsdk:"alert_profile_id"`
	SlaProfileID         types.String `tfsdk:"sla_profile_id"`
	GroupID              types.String `tfsdk:"group_id"`
	Tags                 types.Set    `tfsdk:"tags"`
	Status               types.String `tfsdk:"status"`
	CreatedAt            types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type tcpMonitorCreateBody struct {
	Name                 string  `json:"name"`
	Hostname             string  `json:"hostname"`
	Port                 *int64  `json:"port,omitempty"`
	SendData             *string `json:"sendData,omitempty"`
	ExpectResponse       *string `json:"expectResponse,omitempty"`
	UseTLS               *bool   `json:"useTls,omitempty"`
	CheckIntervalSeconds *int64  `json:"checkIntervalSeconds,omitempty"`
	TimeoutSeconds       *int64  `json:"timeoutSeconds,omitempty"`
	LocationProfileID    *string `json:"locationProfileId,omitempty"`
	ThresholdProfileID   *string `json:"thresholdProfileId,omitempty"`
	AlertProfileID       *string `json:"alertProfileId,omitempty"`
	GroupID              *string `json:"groupId,omitempty"`
	Tags                 *string `json:"tags,omitempty"`
}

// tcpMonitorUpdateBody mirrors the PATCH DTO; pointers omit unchanged fields.
type tcpMonitorUpdateBody = tcpMonitorCreateBody

type tcpMonitorResponse struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Status               string  `json:"status"`
	Hostname             string  `json:"hostname"`
	Port                 int64   `json:"port"`
	SendData             *string `json:"sendData"`
	ExpectResponse       *string `json:"expectResponse"`
	UseTLS               bool    `json:"useTls"`
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

func (r *tcpMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tcp_monitor"
}

func (r *tcpMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *tcpMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A TCP monitor that opens a socket to a host:port, optionally sends a probe " +
			"payload, and validates the response.",
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
				MarkdownDescription: "Hostname or IP to connect to.",
				Required:            true,
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "TCP port to connect to. Defaults to `80`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(80),
			},
			"send_data": schema.StringAttribute{
				MarkdownDescription: "Optional payload sent after the connection opens.",
				Optional:            true,
			},
			"expect_response": schema.StringAttribute{
				MarkdownDescription: "Substring the response must contain for the check to pass.",
				Optional:            true,
			},
			"use_tls": schema.BoolAttribute{
				MarkdownDescription: "Wrap the connection in TLS. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"check_interval_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds between checks. Defaults to `300`. Plans cap the minimum.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(300),
			},
			"timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Connection timeout in seconds. Defaults to `15`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(15),
			},
			"location_profile_id": schema.StringAttribute{
				MarkdownDescription: "Profile that controls which regions run the check.",
				Optional:            true,
			},
			"threshold_profile_id": schema.StringAttribute{
				MarkdownDescription: "Profile that controls latency thresholds and consecutive-failure logic.",
				Optional:            true,
			},
			"sla_profile_id": schema.StringAttribute{
				MarkdownDescription: "SLA profile attached to this monitor for uptime/latency SLO and error-budget tracking. Requires the SLA monitoring feature.",
				Optional:            true,
			},
			"alert_profile_id": schema.StringAttribute{
				MarkdownDescription: "Profile that controls notification channels and routing.",
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

func (m *tcpMonitorModel) toCreateBody(ctx context.Context, diags *diagSink) *tcpMonitorCreateBody {
	return &tcpMonitorCreateBody{
		Name:                 m.Name.ValueString(),
		Hostname:             m.Hostname.ValueString(),
		Port:                 int64Ptr(m.Port),
		SendData:             strPtr(m.SendData),
		ExpectResponse:       strPtr(m.ExpectResponse),
		UseTLS:               boolPtr(m.UseTLS),
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
func (m *tcpMonitorModel) applyResponse(ctx context.Context, resp *tcpMonitorResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Hostname = types.StringValue(resp.Hostname)
	m.Port = types.Int64Value(resp.Port)
	m.SendData = strFromPtr(resp.SendData)
	m.ExpectResponse = strFromPtr(resp.ExpectResponse)
	m.UseTLS = types.BoolValue(resp.UseTLS)
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

func (r *tcpMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tcpMonitorModel
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
	path := r.client.Tenantf("/monitoring", "/monitors/tcp")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating TCP monitor", err.Error())
		return
	}

	if slaIsSet(plan.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, created.ID, plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error attaching SLA profile to TCP monitor", err.Error())
			return
		}
	}

	// Re-read to populate all computed fields from the canonical record.
	var got tcpMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/tcp/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading TCP monitor after create", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *tcpMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tcpMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got tcpMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/tcp/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading TCP monitor", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *tcpMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan tcpMonitorModel
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

	path := r.client.Tenantf("/monitoring", "/monitors/tcp/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating TCP monitor", err.Error())
		return
	}

	var priorState tcpMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
	if !resp.Diagnostics.HasError() && !plan.SlaProfileID.Equal(priorState.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, plan.ID.ValueString(), plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error updating SLA profile on TCP monitor", err.Error())
			return
		}
	}

	var got tcpMonitorResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading TCP monitor after update", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *tcpMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state tcpMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/tcp/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting TCP monitor", err.Error())
	}
}

func (r *tcpMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
