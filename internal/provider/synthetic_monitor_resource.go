package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// This file mirrors http_monitor_resource.go for the synthetic (browser) monitor:
// a typed model, wire DTOs, Schema, and Create/Read/Update/Delete that map the
// model onto the Watchpoint monitoring API synthetic DTOs.

var (
	_ resource.Resource                = (*syntheticMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*syntheticMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*syntheticMonitorResource)(nil)
)

// NewSyntheticMonitorResource is the resource constructor wired into the provider.
func NewSyntheticMonitorResource() resource.Resource { return &syntheticMonitorResource{} }

type syntheticMonitorResource struct {
	client *client.Client
}

// syntheticMonitorModel maps schema attributes <-> Terraform state.
type syntheticMonitorModel struct {
	ID                       types.String  `tfsdk:"id"`
	Name                     types.String  `tfsdk:"name"`
	StartURL                 types.String  `tfsdk:"start_url"`
	ScriptType               types.String  `tfsdk:"script_type"`
	ScriptJSON               types.String  `tfsdk:"script_json"`
	ScriptCode               types.String  `tfsdk:"script_code"`
	ViewportWidth            types.Int64   `tfsdk:"viewport_width"`
	ViewportHeight           types.Int64   `tfsdk:"viewport_height"`
	CaptureScreenshot        types.Bool    `tfsdk:"capture_screenshot"`
	CaptureHar               types.Bool    `tfsdk:"capture_har"`
	DisableCache             types.Bool    `tfsdk:"disable_cache"`
	UserAgent                types.String  `tfsdk:"user_agent"`
	ExtraHeaders             types.String  `tfsdk:"extra_headers"`
	EnableVisualDiff         types.Bool    `tfsdk:"enable_visual_diff"`
	VisualDiffThreshold      types.Float64 `tfsdk:"visual_diff_threshold"`
	ExpectedTexts            types.String  `tfsdk:"expected_texts"`
	TrackWebVitals           types.Bool    `tfsdk:"track_web_vitals"`
	WaitForSelector          types.String  `tfsdk:"wait_for_selector"`
	WaitForSelectorTimeoutMs types.Int64   `tfsdk:"wait_for_selector_timeout_ms"`
	CheckIntervalSeconds     types.Int64   `tfsdk:"check_interval_seconds"`
	TimeoutSeconds           types.Int64   `tfsdk:"timeout_seconds"`
	LocationProfileID        types.String  `tfsdk:"location_profile_id"`
	ThresholdProfileID       types.String  `tfsdk:"threshold_profile_id"`
	AlertProfileID           types.String  `tfsdk:"alert_profile_id"`
	SlaProfileID             types.String  `tfsdk:"sla_profile_id"`
	GroupID                  types.String  `tfsdk:"group_id"`
	Tags                     types.Set     `tfsdk:"tags"`
	Status                   types.String  `tfsdk:"status"`
	CreatedAt                types.String  `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type syntheticMonitorCreateBody struct {
	Name                     string   `json:"name"`
	StartURL                 string   `json:"startUrl"`
	ScriptType               *string  `json:"scriptType,omitempty"`
	ScriptJSON               *string  `json:"scriptJson,omitempty"`
	ScriptCode               *string  `json:"scriptCode,omitempty"`
	ViewportWidth            *int64   `json:"viewportWidth,omitempty"`
	ViewportHeight           *int64   `json:"viewportHeight,omitempty"`
	CaptureScreenshot        *bool    `json:"captureScreenshot,omitempty"`
	CaptureHar               *bool    `json:"captureHar,omitempty"`
	DisableCache             *bool    `json:"disableCache,omitempty"`
	UserAgent                *string  `json:"userAgent,omitempty"`
	ExtraHeaders             *string  `json:"extraHeaders,omitempty"`
	EnableVisualDiff         *bool    `json:"enableVisualDiff,omitempty"`
	VisualDiffThreshold      *float64 `json:"visualDiffThreshold,omitempty"`
	ExpectedTexts            *string  `json:"expectedTexts,omitempty"`
	TrackWebVitals           *bool    `json:"trackWebVitals,omitempty"`
	WaitForSelector          *string  `json:"waitForSelector,omitempty"`
	WaitForSelectorTimeoutMs *int64   `json:"waitForSelectorTimeoutMs,omitempty"`
	CheckIntervalSeconds     *int64   `json:"checkIntervalSeconds,omitempty"`
	TimeoutSeconds           *int64   `json:"timeoutSeconds,omitempty"`
	LocationProfileID        *string  `json:"locationProfileId,omitempty"`
	ThresholdProfileID       *string  `json:"thresholdProfileId,omitempty"`
	AlertProfileID           *string  `json:"alertProfileId,omitempty"`
	GroupID                  *string  `json:"groupId,omitempty"`
	Tags                     *string  `json:"tags,omitempty"`
}

// syntheticMonitorUpdateBody mirrors the PATCH DTO; pointers omit unchanged fields.
type syntheticMonitorUpdateBody = syntheticMonitorCreateBody

type syntheticMonitorResponse struct {
	ID                       string  `json:"id"`
	Name                     string  `json:"name"`
	Status                   string  `json:"status"`
	StartURL                 string  `json:"startUrl"`
	ScriptType               string  `json:"scriptType"`
	ScriptJSON               *string `json:"scriptJson"`
	ScriptCode               *string `json:"scriptCode"`
	ViewportWidth            int64   `json:"viewportWidth"`
	ViewportHeight           int64   `json:"viewportHeight"`
	CaptureScreenshot        bool    `json:"captureScreenshot"`
	CaptureHar               bool    `json:"captureHar"`
	DisableCache             bool    `json:"disableCache"`
	UserAgent                *string `json:"userAgent"`
	ExtraHeaders             *string `json:"extraHeaders"`
	EnableVisualDiff         bool    `json:"enableVisualDiff"`
	VisualDiffThreshold      float64 `json:"visualDiffThreshold"`
	ExpectedTexts            *string `json:"expectedTexts"`
	TrackWebVitals           bool    `json:"trackWebVitals"`
	WaitForSelector          *string `json:"waitForSelector"`
	WaitForSelectorTimeoutMs int64   `json:"waitForSelectorTimeoutMs"`
	CheckIntervalSeconds     int64   `json:"checkIntervalSeconds"`
	TimeoutSeconds           int64   `json:"timeoutSeconds"`
	LocationProfileID        *string `json:"locationProfileId"`
	ThresholdProfileID       *string `json:"thresholdProfileId"`
	AlertProfileID           *string `json:"alertProfileId"`
	SlaProfileID             *string `json:"slaProfileId"`
	GroupID                  *string `json:"groupId"`
	Tags                     *string `json:"tags"`
	CreatedAt                string  `json:"createdAt"`
}

func (r *syntheticMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_synthetic_monitor"
}

func (r *syntheticMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *syntheticMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A synthetic (headless-browser) monitor that loads a page or runs a scripted " +
			"flow, capturing screenshots, web vitals, and optional visual diffs.",
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
			"start_url": schema.StringAttribute{
				MarkdownDescription: "URL the browser navigates to first, including scheme.",
				Required:            true,
			},
			"script_type": schema.StringAttribute{
				MarkdownDescription: "Script flavor: `SimpleUrl`, `Recorded`, or `Code`. Defaults to `SimpleUrl`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("SimpleUrl"),
			},
			"script_json": schema.StringAttribute{
				MarkdownDescription: "JSON-encoded recorded step definitions (for `Recorded` scripts).",
				Optional:            true,
			},
			"script_code": schema.StringAttribute{
				MarkdownDescription: "Raw script source (for `Code` scripts).",
				Optional:            true,
			},
			"viewport_width": schema.Int64Attribute{
				MarkdownDescription: "Browser viewport width in pixels. Defaults to `1920`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1920),
			},
			"viewport_height": schema.Int64Attribute{
				MarkdownDescription: "Browser viewport height in pixels. Defaults to `1080`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1080),
			},
			"capture_screenshot": schema.BoolAttribute{
				MarkdownDescription: "Capture a screenshot on each run. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"capture_har": schema.BoolAttribute{
				MarkdownDescription: "Capture a HAR archive of network activity. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"disable_cache": schema.BoolAttribute{
				MarkdownDescription: "Disable the browser cache for each run. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"user_agent": schema.StringAttribute{
				MarkdownDescription: "Override the browser User-Agent header.",
				Optional:            true,
			},
			"extra_headers": schema.StringAttribute{
				MarkdownDescription: "JSON-encoded extra request headers map.",
				Optional:            true,
			},
			"enable_visual_diff": schema.BoolAttribute{
				MarkdownDescription: "Compare screenshots against a baseline and alert on visual drift. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"visual_diff_threshold": schema.Float64Attribute{
				MarkdownDescription: "Percent of changed pixels that triggers a visual-diff failure. Defaults to `5.0`.",
				Optional:            true,
				Computed:            true,
				Default:             float64default.StaticFloat64(5.0),
			},
			"expected_texts": schema.StringAttribute{
				MarkdownDescription: "Comma-separated text snippets that must be present on the page.",
				Optional:            true,
			},
			"track_web_vitals": schema.BoolAttribute{
				MarkdownDescription: "Collect Core Web Vitals (LCP/FID/CLS). Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"wait_for_selector": schema.StringAttribute{
				MarkdownDescription: "CSS selector to wait for before considering the page loaded.",
				Optional:            true,
			},
			"wait_for_selector_timeout_ms": schema.Int64Attribute{
				MarkdownDescription: "Milliseconds to wait for `wait_for_selector` before failing. Defaults to `5000`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(5000),
			},
			"check_interval_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds between checks. Defaults to `900`. Plans cap the minimum.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(900),
			},
			"timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Run timeout in seconds. Defaults to `60`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(60),
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

func (m *syntheticMonitorModel) toCreateBody(ctx context.Context, diags *diagSink) *syntheticMonitorCreateBody {
	return &syntheticMonitorCreateBody{
		Name:                     m.Name.ValueString(),
		StartURL:                 m.StartURL.ValueString(),
		ScriptType:               strPtr(m.ScriptType),
		ScriptJSON:               strPtr(m.ScriptJSON),
		ScriptCode:               strPtr(m.ScriptCode),
		ViewportWidth:            int64Ptr(m.ViewportWidth),
		ViewportHeight:           int64Ptr(m.ViewportHeight),
		CaptureScreenshot:        boolPtr(m.CaptureScreenshot),
		CaptureHar:               boolPtr(m.CaptureHar),
		DisableCache:             boolPtr(m.DisableCache),
		UserAgent:                strPtr(m.UserAgent),
		ExtraHeaders:             strPtr(m.ExtraHeaders),
		EnableVisualDiff:         boolPtr(m.EnableVisualDiff),
		VisualDiffThreshold:      float64Ptr(m.VisualDiffThreshold),
		ExpectedTexts:            strPtr(m.ExpectedTexts),
		TrackWebVitals:           boolPtr(m.TrackWebVitals),
		WaitForSelector:          strPtr(m.WaitForSelector),
		WaitForSelectorTimeoutMs: int64Ptr(m.WaitForSelectorTimeoutMs),
		CheckIntervalSeconds:     int64Ptr(m.CheckIntervalSeconds),
		TimeoutSeconds:           int64Ptr(m.TimeoutSeconds),
		LocationProfileID:        strPtr(m.LocationProfileID),
		ThresholdProfileID:       strPtr(m.ThresholdProfileID),
		AlertProfileID:           strPtr(m.AlertProfileID),
		GroupID:                  strPtr(m.GroupID),
		Tags:                     csvPtrFromSet(ctx, m.Tags, diags.d),
	}
}

// applyResponse copies API response fields into the model.
func (m *syntheticMonitorModel) applyResponse(ctx context.Context, resp *syntheticMonitorResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.StartURL = types.StringValue(resp.StartURL)
	m.ScriptType = types.StringValue(resp.ScriptType)
	m.ScriptJSON = strFromPtr(resp.ScriptJSON)
	m.ScriptCode = strFromPtr(resp.ScriptCode)
	m.ViewportWidth = types.Int64Value(resp.ViewportWidth)
	m.ViewportHeight = types.Int64Value(resp.ViewportHeight)
	m.CaptureScreenshot = types.BoolValue(resp.CaptureScreenshot)
	m.CaptureHar = types.BoolValue(resp.CaptureHar)
	m.DisableCache = types.BoolValue(resp.DisableCache)
	m.UserAgent = strFromPtr(resp.UserAgent)
	m.ExtraHeaders = strFromPtr(resp.ExtraHeaders)
	m.EnableVisualDiff = types.BoolValue(resp.EnableVisualDiff)
	m.VisualDiffThreshold = types.Float64Value(resp.VisualDiffThreshold)
	m.ExpectedTexts = strFromPtr(resp.ExpectedTexts)
	m.TrackWebVitals = types.BoolValue(resp.TrackWebVitals)
	m.WaitForSelector = strFromPtr(resp.WaitForSelector)
	m.WaitForSelectorTimeoutMs = types.Int64Value(resp.WaitForSelectorTimeoutMs)
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

func (r *syntheticMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan syntheticMonitorModel
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
	path := r.client.Tenantf("/monitoring", "/monitors/synthetic")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating synthetic monitor", err.Error())
		return
	}

	if slaIsSet(plan.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, created.ID, plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error attaching SLA profile to synthetic monitor", err.Error())
			return
		}
	}

	// Re-read to populate all computed fields from the canonical record.
	var got syntheticMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/synthetic/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading synthetic monitor after create", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *syntheticMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state syntheticMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got syntheticMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/synthetic/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading synthetic monitor", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *syntheticMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan syntheticMonitorModel
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

	path := r.client.Tenantf("/monitoring", "/monitors/synthetic/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating synthetic monitor", err.Error())
		return
	}

	var priorState syntheticMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
	if !resp.Diagnostics.HasError() && !plan.SlaProfileID.Equal(priorState.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, plan.ID.ValueString(), plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error updating SLA profile on synthetic monitor", err.Error())
			return
		}
	}

	var got syntheticMonitorResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading synthetic monitor after update", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *syntheticMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state syntheticMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/synthetic/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting synthetic monitor", err.Error())
	}
}

func (r *syntheticMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
