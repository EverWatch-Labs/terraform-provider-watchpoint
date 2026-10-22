package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// This file is the reference implementation other monitor/profile resources
// follow: a typed model, a Schema, and Create/Read/Update/Delete that map the
// model onto the Watchpoint monitoring API request/response DTOs.

var (
	_ resource.Resource                = (*httpMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*httpMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*httpMonitorResource)(nil)
)

// NewHTTPMonitorResource is the resource constructor wired into the provider.
func NewHTTPMonitorResource() resource.Resource { return &httpMonitorResource{} }

type httpMonitorResource struct {
	client *client.Client
}

// httpMonitorModel maps schema attributes <-> Terraform state.
type httpMonitorModel struct {
	ID                           types.String `tfsdk:"id"`
	Name                         types.String `tfsdk:"name"`
	URL                          types.String `tfsdk:"url"`
	Method                       types.String `tfsdk:"method"`
	RequestHeaders               types.String `tfsdk:"request_headers"`
	RequestBody                  types.String `tfsdk:"request_body"`
	RequestContentType           types.String `tfsdk:"request_content_type"`
	FollowRedirects              types.Bool   `tfsdk:"follow_redirects"`
	MaxRedirects                 types.Int64  `tfsdk:"max_redirects"`
	BasicAuthUser                types.String `tfsdk:"basic_auth_user"`
	BasicAuthPassword            types.String `tfsdk:"basic_auth_password"`
	BearerToken                  types.String `tfsdk:"bearer_token"`
	ExpectedStatusCodes          types.String `tfsdk:"expected_status_codes"`
	ExpectedBodyContains         types.String `tfsdk:"expected_body_contains"`
	ExpectedBodyRegex            types.String `tfsdk:"expected_body_regex"`
	ExpectedHeaderContains       types.String `tfsdk:"expected_header_contains"`
	ExpectedResponseTypes        types.String `tfsdk:"expected_response_types"`
	ExpectedResponseTypeSeverity types.String `tfsdk:"expected_response_type_severity"`
	ValidateSSL                  types.Bool   `tfsdk:"validate_ssl"`
	CheckIntervalSeconds         types.Int64  `tfsdk:"check_interval_seconds"`
	TimeoutSeconds               types.Int64  `tfsdk:"timeout_seconds"`
	LocationProfileID            types.String `tfsdk:"location_profile_id"`
	ThresholdProfileID           types.String `tfsdk:"threshold_profile_id"`
	AlertProfileID               types.String `tfsdk:"alert_profile_id"`
	SlaProfileID                 types.String `tfsdk:"sla_profile_id"`
	GroupID                      types.String `tfsdk:"group_id"`
	Tags                         types.Set    `tfsdk:"tags"`
	EnvironmentID                types.String `tfsdk:"environment_id"`
	ContentChangeMode            types.String `tfsdk:"content_change_mode"`
	ContentIgnoreRegex           types.String `tfsdk:"content_ignore_regex"`
	IsMultiStep                  types.Bool   `tfsdk:"is_multi_step"`
	StepsJSON                    types.String `tfsdk:"steps_json"`
	Status                       types.String `tfsdk:"status"`
	CreatedAt                    types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type httpMonitorCreateBody struct {
	Name                         string  `json:"name"`
	URL                          string  `json:"url"`
	Method                       *string `json:"method,omitempty"`
	RequestHeaders               *string `json:"requestHeaders,omitempty"`
	RequestBody                  *string `json:"requestBody,omitempty"`
	RequestContentType           *string `json:"requestContentType,omitempty"`
	FollowRedirects              *bool   `json:"followRedirects,omitempty"`
	MaxRedirects                 *int64  `json:"maxRedirects,omitempty"`
	BasicAuthUser                *string `json:"basicAuthUser,omitempty"`
	BasicAuthPassword            *string `json:"basicAuthPassword,omitempty"`
	BearerToken                  *string `json:"bearerToken,omitempty"`
	ExpectedStatusCodes          *string `json:"expectedStatusCodes,omitempty"`
	ExpectedBodyContains         *string `json:"expectedBodyContains,omitempty"`
	ExpectedBodyRegex            *string `json:"expectedBodyRegex,omitempty"`
	ExpectedHeaderContains       *string `json:"expectedHeaderContains,omitempty"`
	ExpectedResponseTypes        *string `json:"expectedResponseTypes,omitempty"`
	ExpectedResponseTypeSeverity *string `json:"expectedResponseTypeSeverity,omitempty"`
	ValidateSSL                  *bool   `json:"validateSsl,omitempty"`
	CheckIntervalSeconds         *int64  `json:"checkIntervalSeconds,omitempty"`
	TimeoutSeconds               *int64  `json:"timeoutSeconds,omitempty"`
	LocationProfileID            *string `json:"locationProfileId,omitempty"`
	ThresholdProfileID           *string `json:"thresholdProfileId,omitempty"`
	AlertProfileID               *string `json:"alertProfileId,omitempty"`
	GroupID                      *string `json:"groupId,omitempty"`
	Tags                         *string `json:"tags,omitempty"`
	EnvironmentID                *string `json:"environmentId,omitempty"`
	ContentChangeMode            *string `json:"contentChangeMode,omitempty"`
	ContentIgnoreRegex           *string `json:"contentIgnoreRegex,omitempty"`
	IsMultiStep                  *bool   `json:"isMultiStep,omitempty"`
	StepsJSON                    *string `json:"stepsJson,omitempty"`
}

// httpMonitorUpdateBody mirrors the PATCH DTO; pointers omit unchanged fields.
type httpMonitorUpdateBody = httpMonitorCreateBody

type httpMonitorResponse struct {
	ID                           string  `json:"id"`
	Name                         string  `json:"name"`
	Status                       string  `json:"status"`
	URL                          string  `json:"url"`
	Method                       string  `json:"method"`
	RequestHeaders               *string `json:"requestHeaders"`
	RequestBody                  *string `json:"requestBody"`
	RequestContentType           *string `json:"requestContentType"`
	FollowRedirects              bool    `json:"followRedirects"`
	MaxRedirects                 int64   `json:"maxRedirects"`
	BasicAuthUser                *string `json:"basicAuthUser"`
	HasBearerToken               bool    `json:"hasBearerToken"`
	ExpectedStatusCodes          *string `json:"expectedStatusCodes"`
	ExpectedBodyContains         *string `json:"expectedBodyContains"`
	ExpectedBodyRegex            *string `json:"expectedBodyRegex"`
	ExpectedHeaderContains       *string `json:"expectedHeaderContains"`
	ExpectedResponseTypes        *string `json:"expectedResponseTypes"`
	ExpectedResponseTypeSeverity string  `json:"expectedResponseTypeSeverity"`
	ValidateSSL                  bool    `json:"validateSsl"`
	CheckIntervalSeconds         int64   `json:"checkIntervalSeconds"`
	TimeoutSeconds               int64   `json:"timeoutSeconds"`
	LocationProfileID            *string `json:"locationProfileId"`
	ThresholdProfileID           *string `json:"thresholdProfileId"`
	AlertProfileID               *string `json:"alertProfileId"`
	SlaProfileID                 *string `json:"slaProfileId"`
	GroupID                      *string `json:"groupId"`
	Tags                         *string `json:"tags"`
	EnvironmentID                *string `json:"environmentId,omitempty"`
	ContentChangeMode            *string `json:"contentChangeMode,omitempty"`
	ContentIgnoreRegex           *string `json:"contentIgnoreRegex,omitempty"`
	IsMultiStep                  bool    `json:"isMultiStep"`
	StepsJSON                    *string `json:"stepsJson"`
	CreatedAt                    string  `json:"createdAt"`
}

type idNameResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (r *httpMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_http_monitor"
}

func (r *httpMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *httpMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An HTTP/API monitor that checks a URL at a fixed interval with full timing " +
			"breakdowns (DNS / TCP / TLS / TTFB / transfer).",
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
			"url": schema.StringAttribute{
				MarkdownDescription: "Full URL to check, including scheme.",
				Required:            true,
			},
			"method": schema.StringAttribute{
				MarkdownDescription: "HTTP method (GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS). Defaults to `GET`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("GET"),
			},
			"request_headers": schema.StringAttribute{
				MarkdownDescription: "JSON-encoded request headers map.",
				Optional:            true,
			},
			"request_body": schema.StringAttribute{
				MarkdownDescription: "Raw request body sent for POST/PUT/PATCH.",
				Optional:            true,
			},
			"request_content_type": schema.StringAttribute{
				MarkdownDescription: "Content-Type header for the request body.",
				Optional:            true,
			},
			"follow_redirects": schema.BoolAttribute{
				MarkdownDescription: "Follow 3xx redirects. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"max_redirects": schema.Int64Attribute{
				MarkdownDescription: "Maximum redirects to follow. Defaults to `5`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(5),
			},
			"basic_auth_user": schema.StringAttribute{
				MarkdownDescription: "HTTP Basic auth username.",
				Optional:            true,
			},
			"basic_auth_password": schema.StringAttribute{
				MarkdownDescription: "HTTP Basic auth password. Write-only: the API never returns it, so it " +
					"is not refreshed on read.",
				Optional:  true,
				Sensitive: true,
			},
			"bearer_token": schema.StringAttribute{
				MarkdownDescription: "Bearer token sent in the Authorization header. Write-only: the API never " +
					"returns it, so it is not refreshed on read.",
				Optional:  true,
				Sensitive: true,
			},
			"expected_status_codes": schema.StringAttribute{
				MarkdownDescription: "Comma-separated acceptable status codes (e.g. `200,201`).",
				Optional:            true,
				Computed:            true,
			},
			"expected_body_contains": schema.StringAttribute{
				MarkdownDescription: "Substring that must appear in the response body for the check to pass.",
				Optional:            true,
			},
			"expected_body_regex": schema.StringAttribute{
				MarkdownDescription: "Regex the response body must match.",
				Optional:            true,
			},
			"expected_header_contains": schema.StringAttribute{
				MarkdownDescription: "Substring that must appear in the response headers.",
				Optional:            true,
			},
			"expected_response_types": schema.StringAttribute{
				MarkdownDescription: "Comma-separated expected response content types.",
				Optional:            true,
			},
			"expected_response_type_severity": schema.StringAttribute{
				MarkdownDescription: "Severity raised on a response-type mismatch. Defaults to `Warning`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("Warning"),
			},
			"validate_ssl": schema.BoolAttribute{
				MarkdownDescription: "Verify the TLS certificate. When false, self-signed/expired certs are accepted. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"check_interval_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds between checks. Defaults to `300`. Plans cap the minimum.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(300),
			},
			"timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Request timeout in seconds. Defaults to `30`. Must be < check_interval_seconds.",
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
			"environment_id":       environmentIDAttribute(),
			"content_change_mode":  contentChangeModeAttribute(),
			"content_ignore_regex": contentIgnoreRegexAttribute(),
			"tags": schema.SetAttribute{
				MarkdownDescription: "Free-form tags surfaced in filters and SGL queries (`attributes.tags`). Unordered.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"is_multi_step": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a multi-step HTTP pipeline. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"steps_json": schema.StringAttribute{
				MarkdownDescription: "JSON-encoded step definitions for multi-step HTTP checks.",
				Optional:            true,
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

func (m *httpMonitorModel) toCreateBody(ctx context.Context, diags *diagSink) *httpMonitorCreateBody {
	return &httpMonitorCreateBody{
		Name:                         m.Name.ValueString(),
		URL:                          m.URL.ValueString(),
		Method:                       strPtr(m.Method),
		RequestHeaders:               strPtr(m.RequestHeaders),
		RequestBody:                  strPtr(m.RequestBody),
		RequestContentType:           strPtr(m.RequestContentType),
		FollowRedirects:              boolPtr(m.FollowRedirects),
		MaxRedirects:                 int64Ptr(m.MaxRedirects),
		BasicAuthUser:                strPtr(m.BasicAuthUser),
		BasicAuthPassword:            strPtr(m.BasicAuthPassword),
		BearerToken:                  strPtr(m.BearerToken),
		ExpectedStatusCodes:          strPtr(m.ExpectedStatusCodes),
		ExpectedBodyContains:         strPtr(m.ExpectedBodyContains),
		ExpectedBodyRegex:            strPtr(m.ExpectedBodyRegex),
		ExpectedHeaderContains:       strPtr(m.ExpectedHeaderContains),
		ExpectedResponseTypes:        strPtr(m.ExpectedResponseTypes),
		ExpectedResponseTypeSeverity: strPtr(m.ExpectedResponseTypeSeverity),
		ValidateSSL:                  boolPtr(m.ValidateSSL),
		CheckIntervalSeconds:         int64Ptr(m.CheckIntervalSeconds),
		TimeoutSeconds:               int64Ptr(m.TimeoutSeconds),
		LocationProfileID:            strPtr(m.LocationProfileID),
		ThresholdProfileID:           strPtr(m.ThresholdProfileID),
		AlertProfileID:               strPtr(m.AlertProfileID),
		GroupID:                      strPtr(m.GroupID),
		Tags:                         csvPtrFromSet(ctx, m.Tags, diags.d),
		EnvironmentID:                strPtr(m.EnvironmentID),
		ContentChangeMode:            strPtr(m.ContentChangeMode),
		ContentIgnoreRegex:           strPtr(m.ContentIgnoreRegex),
		IsMultiStep:                  boolPtr(m.IsMultiStep),
		StepsJSON:                    strPtr(m.StepsJSON),
	}
}

// applyResponse copies API response fields into the model. Write-only secrets
// (basic_auth_password, bearer_token) are intentionally left untouched.
func (m *httpMonitorModel) applyResponse(ctx context.Context, resp *httpMonitorResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.URL = types.StringValue(resp.URL)
	m.Method = types.StringValue(resp.Method)
	m.RequestHeaders = strFromPtr(resp.RequestHeaders)
	m.RequestBody = strFromPtr(resp.RequestBody)
	m.RequestContentType = strFromPtr(resp.RequestContentType)
	m.FollowRedirects = types.BoolValue(resp.FollowRedirects)
	m.MaxRedirects = types.Int64Value(resp.MaxRedirects)
	m.BasicAuthUser = strFromPtr(resp.BasicAuthUser)
	m.ExpectedStatusCodes = strFromPtr(resp.ExpectedStatusCodes)
	m.ExpectedBodyContains = strFromPtr(resp.ExpectedBodyContains)
	m.ExpectedBodyRegex = strFromPtr(resp.ExpectedBodyRegex)
	m.ExpectedHeaderContains = strFromPtr(resp.ExpectedHeaderContains)
	m.ExpectedResponseTypes = strFromPtr(resp.ExpectedResponseTypes)
	m.ExpectedResponseTypeSeverity = types.StringValue(resp.ExpectedResponseTypeSeverity)
	m.ValidateSSL = types.BoolValue(resp.ValidateSSL)
	m.CheckIntervalSeconds = types.Int64Value(resp.CheckIntervalSeconds)
	m.TimeoutSeconds = types.Int64Value(resp.TimeoutSeconds)
	m.LocationProfileID = strFromPtr(resp.LocationProfileID)
	m.ThresholdProfileID = strFromPtr(resp.ThresholdProfileID)
	m.AlertProfileID = strFromPtr(resp.AlertProfileID)
	m.SlaProfileID = strFromPtr(resp.SlaProfileID)
	m.GroupID = strFromPtr(resp.GroupID)
	m.Tags = setFromCSVPtr(ctx, resp.Tags, diags.d)
	m.EnvironmentID = strFromPtr(resp.EnvironmentID)
	m.ContentChangeMode = strFromPtr(resp.ContentChangeMode)
	m.ContentIgnoreRegex = strFromPtr(resp.ContentIgnoreRegex)
	m.IsMultiStep = types.BoolValue(resp.IsMultiStep)
	m.StepsJSON = strFromPtr(resp.StepsJSON)
	m.Status = types.StringValue(resp.Status)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
}

func (r *httpMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan httpMonitorModel
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
	path := r.client.Tenantf("/monitoring", "/monitors/http")
	if err := r.client.PostManaged(ctx, path, body, &created, monitorAddress("http")); err != nil {
		resp.Diagnostics.AddError("Error creating HTTP monitor", err.Error())
		return
	}

	// Re-read to populate all computed fields from the canonical record.
	var got httpMonitorResponse
	if slaIsSet(plan.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, monitorAddress("http"), created.ID, plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error attaching SLA profile to http monitor", err.Error())
			return
		}
	}
	getPath := r.client.Tenantf("/monitoring", "/monitors/http/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading HTTP monitor after create", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *httpMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state httpMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got httpMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/http/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading HTTP monitor", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *httpMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan httpMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toCreateBody(ctx, ds)
	// An emptied tag set has to reach the API as "" or the old tags survive the PATCH.
	body.Tags = csvPtrClearing(ctx, plan.Tags, &resp.Diagnostics)
	body.ContentIgnoreRegex = strPtrClearing(plan.ContentIgnoreRegex)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/http/%s", plan.ID.ValueString())
	if err := r.client.PatchManaged(ctx, path, body, nil, monitorAddress("http")); err != nil {
		resp.Diagnostics.AddError("Error updating HTTP monitor", err.Error())
		return
	}

	var priorState httpMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
	if !resp.Diagnostics.HasError() && !plan.SlaProfileID.Equal(priorState.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, monitorAddress("http"), plan.ID.ValueString(), plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error updating SLA profile on HTTP monitor", err.Error())
			return
		}
	}

	var got httpMonitorResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading HTTP monitor after update", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *httpMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state httpMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/http/%s", state.ID.ValueString())
	if err := r.client.DeleteManaged(ctx, path, monitorAddress("http")); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting HTTP monitor", err.Error())
	}
}

func (r *httpMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importManagedMonitor(ctx, r.client, "http", req, resp)
}
