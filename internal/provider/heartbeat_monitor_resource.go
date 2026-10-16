package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// This file mirrors ping_monitor_resource.go for the heartbeat (cron /
// dead-man's-switch) monitor. A heartbeat is never probed: the customer's job
// pings an unguessable URL, and the monitor fails when a ping does not arrive
// within expected_period_seconds + grace_period_seconds. It therefore has no
// target, no check_interval_seconds and no timeout_seconds; the server mirrors
// the expected period onto the shared check-interval column itself.
//
// The ping token is the sole credential on an anonymous ingest endpoint, so it
// is server-minted (never accepted from a client), marked Sensitive here, and
// rotated through rotate_trigger the way watchpoint_api_key rotates a secret.

var (
	_ resource.Resource                = (*heartbeatMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*heartbeatMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*heartbeatMonitorResource)(nil)
)

// Bounds the API enforces on both timings; validated here so a typo fails at
// plan time rather than as a 400 half way through an apply.
const (
	minHeartbeatPeriodSeconds = 60
	maxHeartbeatPeriodSeconds = 2592000 // 30 days
)

// NewHeartbeatMonitorResource is the resource constructor wired into the provider.
func NewHeartbeatMonitorResource() resource.Resource { return &heartbeatMonitorResource{} }

type heartbeatMonitorResource struct {
	client *client.Client
}

// heartbeatMonitorModel maps schema attributes <-> Terraform state.
type heartbeatMonitorModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	ExpectedPeriodSeconds types.Int64  `tfsdk:"expected_period_seconds"`
	GracePeriodSeconds    types.Int64  `tfsdk:"grace_period_seconds"`
	LocationProfileID     types.String `tfsdk:"location_profile_id"`
	ThresholdProfileID    types.String `tfsdk:"threshold_profile_id"`
	AlertProfileID        types.String `tfsdk:"alert_profile_id"`
	GroupID               types.String `tfsdk:"group_id"`
	Tags                  types.Set    `tfsdk:"tags"`
	RotateTrigger         types.String `tfsdk:"rotate_trigger"`
	PingToken             types.String `tfsdk:"ping_token"`
	PingURL               types.String `tfsdk:"ping_url"`
	SlaProfileID          types.String `tfsdk:"sla_profile_id"`
	CronExpression        types.String `tfsdk:"cron_expression"`
	TimeZone              types.String `tfsdk:"time_zone"`
	NextExpectedAt        types.String `tfsdk:"next_expected_at"`
	Status                types.String `tfsdk:"status"`
	LastCheckAt           types.String `tfsdk:"last_check_at"`
	LastCheckStatus       types.String `tfsdk:"last_check_status"`
	CreatedAt             types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type heartbeatMonitorCreateBody struct {
	Name                  string  `json:"name"`
	ExpectedPeriodSeconds int64   `json:"expectedPeriodSeconds"`
	GracePeriodSeconds    int64   `json:"gracePeriodSeconds"`
	LocationProfileID     *string `json:"locationProfileId,omitempty"`
	ThresholdProfileID    *string `json:"thresholdProfileId,omitempty"`
	AlertProfileID        *string `json:"alertProfileId,omitempty"`
	GroupID               *string `json:"groupId,omitempty"`
	Tags                  *string `json:"tags,omitempty"`
	CronExpression        *string `json:"cronExpression,omitempty"`
	TimeZone              *string `json:"timeZone,omitempty"`
}

// heartbeatMonitorUpdateBody mirrors the PATCH DTO. The profile ids and tags are
// sent even when empty (as ""), because the API reads "" as "clear this" and an
// omitted field as "leave unchanged": omitting them would silently ignore an
// attribute the operator deleted from the configuration.
type heartbeatMonitorUpdateBody struct {
	Name                  *string `json:"name,omitempty"`
	ExpectedPeriodSeconds *int64  `json:"expectedPeriodSeconds,omitempty"`
	GracePeriodSeconds    *int64  `json:"gracePeriodSeconds,omitempty"`
	LocationProfileID     *string `json:"locationProfileId"`
	ThresholdProfileID    *string `json:"thresholdProfileId"`
	AlertProfileID        *string `json:"alertProfileId"`
	GroupID               *string `json:"groupId"`
	Tags                  *string `json:"tags"`
	CronExpression        *string `json:"cronExpression"`
	TimeZone              *string `json:"timeZone"`
	RotateToken           *bool   `json:"rotateToken,omitempty"`
}

// heartbeatCreatedResponse is the 201 shape: the ping URL is returned there
// because it is the only thing the customer has to act on next.
type heartbeatCreatedResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PingToken string `json:"pingToken"`
	PingURL   string `json:"pingUrl"`
}

type heartbeatMonitorResponse struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	Status                string  `json:"status"`
	PingToken             string  `json:"pingToken"`
	PingURL               string  `json:"pingUrl"`
	ExpectedPeriodSeconds int64   `json:"expectedPeriodSeconds"`
	GracePeriodSeconds    int64   `json:"gracePeriodSeconds"`
	LocationProfileID     *string `json:"locationProfileId"`
	ThresholdProfileID    *string `json:"thresholdProfileId"`
	AlertProfileID        *string `json:"alertProfileId"`
	GroupID               *string `json:"groupId"`
	Tags                  *string `json:"tags"`
	LastCheckAt           *string `json:"lastCheckAt"`
	LastCheckStatus       *string `json:"lastCheckStatus"`
	CreatedAt             string  `json:"createdAt"`
	SlaProfileID          *string `json:"slaProfileId"`
	CronExpression        *string `json:"cronExpression"`
	TimeZone              *string `json:"timeZone"`
	NextExpectedAt        *string `json:"nextExpectedAt"`
}

func (r *heartbeatMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_heartbeat_monitor"
}

func (r *heartbeatMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *heartbeatMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A heartbeat (cron / dead-man's-switch) monitor. Nothing is probed: a scheduled " +
			"job pings `ping_url` when it succeeds, and the monitor fails when no ping arrives within " +
			"`expected_period_seconds` + `grace_period_seconds`. Requires the `heartbeat_monitors` feature " +
			"(any paid plan).",
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
			"expected_period_seconds": schema.Int64Attribute{
				MarkdownDescription: "How often the job is expected to ping, in seconds. Defaults to `3600`. " +
					"Between `60` and `2592000` (30 days).",
				Optional:   true,
				Computed:   true,
				Default:    int64default.StaticInt64(3600),
				Validators: []validator.Int64{int64validator.Between(minHeartbeatPeriodSeconds, maxHeartbeatPeriodSeconds)},
			},
			"grace_period_seconds": schema.Int64Attribute{
				MarkdownDescription: "Slack allowed on top of `expected_period_seconds` before the monitor is " +
					"marked down, in seconds. Defaults to `300`. Between `0` and `2592000`.",
				Optional:   true,
				Computed:   true,
				Default:    int64default.StaticInt64(300),
				Validators: []validator.Int64{int64validator.Between(0, maxHeartbeatPeriodSeconds)},
			},
			"location_profile_id": schema.StringAttribute{
				MarkdownDescription: "Location profile recorded on the monitor. A heartbeat is not probed from " +
					"a region, so this only labels where the work is expected to run.",
				Optional: true,
			},
			"threshold_profile_id": schema.StringAttribute{
				MarkdownDescription: "Profile that controls consecutive-failure logic.",
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
			"rotate_trigger": schema.StringAttribute{
				MarkdownDescription: "Arbitrary value that rotates `ping_token` when it changes. Change it (for " +
					"example to a new date) to mint a fresh token in place, keeping the same `id`. The previous " +
					"`ping_url` stops working immediately, so every job using it has to be updated in the same apply.",
				Optional: true,
			},
			"ping_token": schema.StringAttribute{
				MarkdownDescription: "Server-minted 43-character token that authenticates the ping. It is the " +
					"only credential on an anonymous endpoint, so treat it as a secret: anyone holding it can " +
					"report this job as healthy.",
				Computed:  true,
				Sensitive: true,
			},
			"ping_url": schema.StringAttribute{
				MarkdownDescription: "Fully-formed URL the job calls on success (`GET` or `POST`). Append " +
					"`/start` to record a start time and `/fail` to fail the check immediately without waiting " +
					"out the grace period. Embeds `ping_token`, so it is equally sensitive.",
				Computed:  true,
				Sensitive: true,
			},
			"sla_profile_id": schema.StringAttribute{
				MarkdownDescription: "SLA profile attached to this monitor. When omitted the monitor keeps " +
					"what it has, which on create is the tenant's default SLA profile if one is set.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cron_expression": schema.StringAttribute{
				MarkdownDescription: "Five-field cron schedule the job runs on, for example `0 2 * * 1-5`. When set " +
					"it replaces `expected_period_seconds` for overdue detection: a ping is expected at each " +
					"scheduled run plus `grace_period_seconds`, so a weekday job stays quiet at the weekend. " +
					"Six-field (seconds) cron is rejected.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.LengthAtMost(128)},
			},
			"time_zone": schema.StringAttribute{
				MarkdownDescription: "IANA time zone `cron_expression` is evaluated in, for example " +
					"`Europe/Berlin`. Defaults to UTC. Requires `cron_expression`.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.LengthAtMost(64), stringvalidator.AlsoRequires(pathRootExpr("cron_expression"))},
			},
			"next_expected_at": schema.StringAttribute{
				MarkdownDescription: "Next scheduled run the monitor is waiting on (RFC 3339, UTC), for cron " +
					"schedules; null for a fixed period.",
				Computed: true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current monitor status (e.g. `Active`, `Paused`). Only `Active` monitors " +
					"are swept for being overdue; the others keep accepting pings without paging.",
				Computed: true,
			},
			"last_check_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the last ping received (RFC 3339), or null while the monitor " +
					"is still waiting for its first ping. A monitor that has never pinged is never marked down.",
				Computed: true,
			},
			"last_check_status": schema.StringAttribute{
				MarkdownDescription: "Status of the last recorded check (`Success`, `Critical`, ...), or null.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *heartbeatMonitorModel) toCreateBody(ctx context.Context, diags *diagSink) *heartbeatMonitorCreateBody {
	return &heartbeatMonitorCreateBody{
		Name:                  m.Name.ValueString(),
		ExpectedPeriodSeconds: m.ExpectedPeriodSeconds.ValueInt64(),
		GracePeriodSeconds:    m.GracePeriodSeconds.ValueInt64(),
		LocationProfileID:     strPtr(m.LocationProfileID),
		ThresholdProfileID:    strPtr(m.ThresholdProfileID),
		AlertProfileID:        strPtr(m.AlertProfileID),
		GroupID:               strPtr(m.GroupID),
		Tags:                  csvPtrFromSet(ctx, m.Tags, diags.d),
		CronExpression:        strPtr(m.CronExpression),
		TimeZone:              strPtr(m.TimeZone),
	}
}

// toUpdateBody builds the PATCH body. rotate is true when rotate_trigger changed.
func (m *heartbeatMonitorModel) toUpdateBody(ctx context.Context, rotate bool, diags *diagSink) *heartbeatMonitorUpdateBody {
	body := &heartbeatMonitorUpdateBody{
		Name:                  strPtr(m.Name),
		ExpectedPeriodSeconds: int64Ptr(m.ExpectedPeriodSeconds),
		GracePeriodSeconds:    int64Ptr(m.GracePeriodSeconds),
		LocationProfileID:     strPtrClearing(m.LocationProfileID),
		ThresholdProfileID:    strPtrClearing(m.ThresholdProfileID),
		AlertProfileID:        strPtrClearing(m.AlertProfileID),
		GroupID:               strPtrClearing(m.GroupID),
		Tags:                  csvPtrClearing(ctx, m.Tags, diags.d),
		// "" clears the schedule back to the fixed period; null would keep it.
		CronExpression: strPtrClearing(m.CronExpression),
		TimeZone:       strPtrClearing(m.TimeZone),
	}
	if rotate {
		body.RotateToken = &rotate
	}
	return body
}

// applyResponse copies API response fields into the model.
func (m *heartbeatMonitorModel) applyResponse(ctx context.Context, resp *heartbeatMonitorResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.ExpectedPeriodSeconds = types.Int64Value(resp.ExpectedPeriodSeconds)
	m.GracePeriodSeconds = types.Int64Value(resp.GracePeriodSeconds)
	m.LocationProfileID = strFromPtr(resp.LocationProfileID)
	m.ThresholdProfileID = strFromPtr(resp.ThresholdProfileID)
	m.AlertProfileID = strFromPtr(resp.AlertProfileID)
	m.GroupID = strFromPtr(resp.GroupID)
	m.Tags = setFromCSVPtr(ctx, resp.Tags, diags.d)
	m.PingToken = types.StringValue(resp.PingToken)
	m.PingURL = types.StringValue(resp.PingURL)
	m.SlaProfileID = strFromPtr(resp.SlaProfileID)
	m.CronExpression = strFromPtr(resp.CronExpression)
	m.TimeZone = strFromPtr(resp.TimeZone)
	m.NextExpectedAt = strFromPtr(resp.NextExpectedAt)
	m.Status = types.StringValue(resp.Status)
	m.LastCheckAt = strFromPtr(resp.LastCheckAt)
	m.LastCheckStatus = strFromPtr(resp.LastCheckStatus)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
}

func (r *heartbeatMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan heartbeatMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toCreateBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var created heartbeatCreatedResponse
	path := r.client.Tenantf("/monitoring", "/monitors/heartbeat")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating heartbeat monitor", err.Error())
		return
	}

	if slaIsSet(plan.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, created.ID, plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error attaching SLA profile to heartbeat monitor", err.Error())
			return
		}
	}

	// Re-read to populate all computed fields from the canonical record.
	var got heartbeatMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/heartbeat/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading heartbeat monitor after create", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *heartbeatMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state heartbeatMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got heartbeatMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/heartbeat/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading heartbeat monitor", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *heartbeatMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state heartbeatMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toUpdateBody(ctx, !plan.RotateTrigger.Equal(state.RotateTrigger), ds)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/heartbeat/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating heartbeat monitor", err.Error())
		return
	}
	if !plan.SlaProfileID.Equal(state.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, plan.ID.ValueString(), plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error updating SLA profile on heartbeat monitor", err.Error())
			return
		}
	}

	var got heartbeatMonitorResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading heartbeat monitor after update", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *heartbeatMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state heartbeatMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/heartbeat/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting heartbeat monitor", err.Error())
	}
}

func (r *heartbeatMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
