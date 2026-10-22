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

// This file mirrors http_monitor_resource.go for the SSL/TLS certificate
// monitor: a typed model, wire DTOs, Schema, and Create/Read/Update/Delete that
// map the model onto the Watchpoint monitoring API SSL DTOs.

var (
	_ resource.Resource                = (*sslMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*sslMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*sslMonitorResource)(nil)
)

// NewSSLMonitorResource is the resource constructor wired into the provider.
func NewSSLMonitorResource() resource.Resource { return &sslMonitorResource{} }

type sslMonitorResource struct {
	client *client.Client
}

// sslMonitorModel maps schema attributes <-> Terraform state.
type sslMonitorModel struct {
	ID                      types.String `tfsdk:"id"`
	Name                    types.String `tfsdk:"name"`
	Hostname                types.String `tfsdk:"hostname"`
	Port                    types.Int64  `tfsdk:"port"`
	ExpiryWarningDays       types.Int64  `tfsdk:"expiry_warning_days"`
	ExpiryCriticalDays      types.Int64  `tfsdk:"expiry_critical_days"`
	CheckCertChain          types.Bool   `tfsdk:"check_cert_chain"`
	CheckRevocation         types.Bool   `tfsdk:"check_revocation"`
	ExpectedIssuer          types.String `tfsdk:"expected_issuer"`
	ExpectedSubjectAltNames types.String `tfsdk:"expected_subject_alt_names"`
	AlertOnCertChange       types.Bool   `tfsdk:"alert_on_cert_change"`
	CheckIntervalSeconds    types.Int64  `tfsdk:"check_interval_seconds"`
	TimeoutSeconds          types.Int64  `tfsdk:"timeout_seconds"`
	LocationProfileID       types.String `tfsdk:"location_profile_id"`
	ThresholdProfileID      types.String `tfsdk:"threshold_profile_id"`
	AlertProfileID          types.String `tfsdk:"alert_profile_id"`
	SlaProfileID            types.String `tfsdk:"sla_profile_id"`
	GroupID                 types.String `tfsdk:"group_id"`
	Tags                    types.Set    `tfsdk:"tags"`
	EnvironmentID           types.String `tfsdk:"environment_id"`
	Status                  types.String `tfsdk:"status"`
	CreatedAt               types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type sslMonitorCreateBody struct {
	Name                    string  `json:"name"`
	Hostname                string  `json:"hostname"`
	Port                    *int64  `json:"port,omitempty"`
	ExpiryWarningDays       *int64  `json:"expiryWarningDays,omitempty"`
	ExpiryCriticalDays      *int64  `json:"expiryCriticalDays,omitempty"`
	CheckCertChain          *bool   `json:"checkCertChain,omitempty"`
	CheckRevocation         *bool   `json:"checkRevocation,omitempty"`
	ExpectedIssuer          *string `json:"expectedIssuer,omitempty"`
	ExpectedSubjectAltNames *string `json:"expectedSubjectAltNames,omitempty"`
	AlertOnCertChange       *bool   `json:"alertOnCertChange,omitempty"`
	CheckIntervalSeconds    *int64  `json:"checkIntervalSeconds,omitempty"`
	TimeoutSeconds          *int64  `json:"timeoutSeconds,omitempty"`
	LocationProfileID       *string `json:"locationProfileId,omitempty"`
	ThresholdProfileID      *string `json:"thresholdProfileId,omitempty"`
	AlertProfileID          *string `json:"alertProfileId,omitempty"`
	GroupID                 *string `json:"groupId,omitempty"`
	Tags                    *string `json:"tags,omitempty"`
	EnvironmentID           *string `json:"environmentId,omitempty"`
}

// sslMonitorUpdateBody mirrors the PATCH DTO; pointers omit unchanged fields.
type sslMonitorUpdateBody = sslMonitorCreateBody

type sslMonitorResponse struct {
	ID                      string  `json:"id"`
	Name                    string  `json:"name"`
	Status                  string  `json:"status"`
	Hostname                string  `json:"hostname"`
	Port                    int64   `json:"port"`
	ExpiryWarningDays       int64   `json:"expiryWarningDays"`
	ExpiryCriticalDays      int64   `json:"expiryCriticalDays"`
	CheckCertChain          bool    `json:"checkCertChain"`
	CheckRevocation         bool    `json:"checkRevocation"`
	ExpectedIssuer          *string `json:"expectedIssuer"`
	ExpectedSubjectAltNames *string `json:"expectedSubjectAltNames"`
	AlertOnCertChange       bool    `json:"alertOnCertChange"`
	CheckIntervalSeconds    int64   `json:"checkIntervalSeconds"`
	TimeoutSeconds          int64   `json:"timeoutSeconds"`
	LocationProfileID       *string `json:"locationProfileId"`
	ThresholdProfileID      *string `json:"thresholdProfileId"`
	AlertProfileID          *string `json:"alertProfileId"`
	SlaProfileID            *string `json:"slaProfileId"`
	GroupID                 *string `json:"groupId"`
	Tags                    *string `json:"tags"`
	EnvironmentID           *string `json:"environmentId,omitempty"`
	CreatedAt               string  `json:"createdAt"`
}

func (r *sslMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssl_monitor"
}

func (r *sslMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *sslMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An SSL/TLS certificate monitor that connects to a host:port, validates the " +
			"certificate, and alerts before it expires.",
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
				MarkdownDescription: "Hostname whose certificate is checked (no scheme).",
				Required:            true,
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "TLS port to connect to. Defaults to `443`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(443),
			},
			"expiry_warning_days": schema.Int64Attribute{
				MarkdownDescription: "Days before expiry at which a warning is raised. Defaults to `30`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(30),
			},
			"expiry_critical_days": schema.Int64Attribute{
				MarkdownDescription: "Days before expiry at which a critical alert is raised. Defaults to `7`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(7),
			},
			"check_cert_chain": schema.BoolAttribute{
				MarkdownDescription: "Validate the full certificate chain. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"check_revocation": schema.BoolAttribute{
				MarkdownDescription: "Check certificate revocation status (OCSP/CRL). Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"expected_issuer": schema.StringAttribute{
				MarkdownDescription: "Issuer the certificate must be issued by; a mismatch fails the check.",
				Optional:            true,
			},
			"expected_subject_alt_names": schema.StringAttribute{
				MarkdownDescription: "Comma-separated subject alternative names the certificate must include.",
				Optional:            true,
			},
			"alert_on_cert_change": schema.BoolAttribute{
				MarkdownDescription: "Raise an alert whenever the certificate changes. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"check_interval_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds between checks. Defaults to `3600`. Plans cap the minimum.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(3600),
			},
			"timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Connection timeout in seconds. Defaults to `30`.",
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
			"environment_id": environmentIDAttribute(),
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

func (m *sslMonitorModel) toCreateBody(ctx context.Context, diags *diagSink) *sslMonitorCreateBody {
	return &sslMonitorCreateBody{
		Name:                    m.Name.ValueString(),
		Hostname:                m.Hostname.ValueString(),
		Port:                    int64Ptr(m.Port),
		ExpiryWarningDays:       int64Ptr(m.ExpiryWarningDays),
		ExpiryCriticalDays:      int64Ptr(m.ExpiryCriticalDays),
		CheckCertChain:          boolPtr(m.CheckCertChain),
		CheckRevocation:         boolPtr(m.CheckRevocation),
		ExpectedIssuer:          strPtr(m.ExpectedIssuer),
		ExpectedSubjectAltNames: strPtr(m.ExpectedSubjectAltNames),
		AlertOnCertChange:       boolPtr(m.AlertOnCertChange),
		CheckIntervalSeconds:    int64Ptr(m.CheckIntervalSeconds),
		TimeoutSeconds:          int64Ptr(m.TimeoutSeconds),
		LocationProfileID:       strPtr(m.LocationProfileID),
		ThresholdProfileID:      strPtr(m.ThresholdProfileID),
		AlertProfileID:          strPtr(m.AlertProfileID),
		GroupID:                 strPtr(m.GroupID),
		Tags:                    csvPtrFromSet(ctx, m.Tags, diags.d),
		EnvironmentID:           strPtr(m.EnvironmentID),
	}
}

// applyResponse copies API response fields into the model.
func (m *sslMonitorModel) applyResponse(ctx context.Context, resp *sslMonitorResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Hostname = types.StringValue(resp.Hostname)
	m.Port = types.Int64Value(resp.Port)
	m.ExpiryWarningDays = types.Int64Value(resp.ExpiryWarningDays)
	m.ExpiryCriticalDays = types.Int64Value(resp.ExpiryCriticalDays)
	m.CheckCertChain = types.BoolValue(resp.CheckCertChain)
	m.CheckRevocation = types.BoolValue(resp.CheckRevocation)
	m.ExpectedIssuer = strFromPtr(resp.ExpectedIssuer)
	m.ExpectedSubjectAltNames = strFromPtr(resp.ExpectedSubjectAltNames)
	m.AlertOnCertChange = types.BoolValue(resp.AlertOnCertChange)
	m.CheckIntervalSeconds = types.Int64Value(resp.CheckIntervalSeconds)
	m.TimeoutSeconds = types.Int64Value(resp.TimeoutSeconds)
	m.LocationProfileID = strFromPtr(resp.LocationProfileID)
	m.ThresholdProfileID = strFromPtr(resp.ThresholdProfileID)
	m.AlertProfileID = strFromPtr(resp.AlertProfileID)
	m.SlaProfileID = strFromPtr(resp.SlaProfileID)
	m.GroupID = strFromPtr(resp.GroupID)
	m.Tags = setFromCSVPtr(ctx, resp.Tags, diags.d)
	m.EnvironmentID = strFromPtr(resp.EnvironmentID)
	m.Status = types.StringValue(resp.Status)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
}

func (r *sslMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sslMonitorModel
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
	path := r.client.Tenantf("/monitoring", "/monitors/ssl")
	if err := r.client.PostManaged(ctx, path, body, &created, monitorAddress("ssl")); err != nil {
		resp.Diagnostics.AddError("Error creating SSL monitor", err.Error())
		return
	}

	// Re-read to populate all computed fields from the canonical record.
	var got sslMonitorResponse
	if slaIsSet(plan.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, monitorAddress("ssl"), created.ID, plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error attaching SLA profile to SSL monitor", err.Error())
			return
		}
	}
	getPath := r.client.Tenantf("/monitoring", "/monitors/ssl/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading SSL monitor after create", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sslMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sslMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got sslMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/ssl/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading SSL monitor", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *sslMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sslMonitorModel
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

	path := r.client.Tenantf("/monitoring", "/monitors/ssl/%s", plan.ID.ValueString())
	if err := r.client.PatchManaged(ctx, path, body, nil, monitorAddress("ssl")); err != nil {
		resp.Diagnostics.AddError("Error updating SSL monitor", err.Error())
		return
	}

	var priorState sslMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
	if !resp.Diagnostics.HasError() && !plan.SlaProfileID.Equal(priorState.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, monitorAddress("ssl"), plan.ID.ValueString(), plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error updating SLA profile on SSL monitor", err.Error())
			return
		}
	}

	var got sslMonitorResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading SSL monitor after update", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sslMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sslMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/ssl/%s", state.ID.ValueString())
	if err := r.client.DeleteManaged(ctx, path, monitorAddress("ssl")); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting SSL monitor", err.Error())
	}
}

func (r *sslMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importManagedMonitor(ctx, r.client, "ssl", req, resp)
}
