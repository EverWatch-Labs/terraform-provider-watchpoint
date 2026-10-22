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

// This file mirrors http_monitor_resource.go for the DNS record monitor: a typed
// model, wire DTOs, Schema, and Create/Read/Update/Delete that map the model onto
// the Watchpoint monitoring API DNS DTOs.

var (
	_ resource.Resource                = (*dnsMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*dnsMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*dnsMonitorResource)(nil)
)

// NewDNSMonitorResource is the resource constructor wired into the provider.
func NewDNSMonitorResource() resource.Resource { return &dnsMonitorResource{} }

type dnsMonitorResource struct {
	client *client.Client
}

// dnsMonitorModel maps schema attributes <-> Terraform state.
type dnsMonitorModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Hostname             types.String `tfsdk:"hostname"`
	RecordType           types.String `tfsdk:"record_type"`
	Nameserver           types.String `tfsdk:"nameserver"`
	ExpectedValues       types.String `tfsdk:"expected_values"`
	AlertOnChange        types.Bool   `tfsdk:"alert_on_change"`
	RequireDnssec        types.Bool   `tfsdk:"require_dnssec"`
	CompareNameservers   types.Bool   `tfsdk:"compare_nameservers"`
	LastResolvedValues   types.String `tfsdk:"last_resolved_values"`
	CheckIntervalSeconds types.Int64  `tfsdk:"check_interval_seconds"`
	TimeoutSeconds       types.Int64  `tfsdk:"timeout_seconds"`
	LocationProfileID    types.String `tfsdk:"location_profile_id"`
	ThresholdProfileID   types.String `tfsdk:"threshold_profile_id"`
	AlertProfileID       types.String `tfsdk:"alert_profile_id"`
	SlaProfileID         types.String `tfsdk:"sla_profile_id"`
	GroupID              types.String `tfsdk:"group_id"`
	Tags                 types.Set    `tfsdk:"tags"`
	EnvironmentID        types.String `tfsdk:"environment_id"`
	Status               types.String `tfsdk:"status"`
	CreatedAt            types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type dnsMonitorCreateBody struct {
	Name                 string  `json:"name"`
	Hostname             string  `json:"hostname"`
	RecordType           *string `json:"recordType,omitempty"`
	Nameserver           *string `json:"nameserver,omitempty"`
	ExpectedValues       *string `json:"expectedValues,omitempty"`
	AlertOnChange        *bool   `json:"alertOnChange,omitempty"`
	RequireDnssec        *bool   `json:"requireDnssec,omitempty"`
	CompareNameservers   *bool   `json:"compareNameservers,omitempty"`
	CheckIntervalSeconds *int64  `json:"checkIntervalSeconds,omitempty"`
	TimeoutSeconds       *int64  `json:"timeoutSeconds,omitempty"`
	LocationProfileID    *string `json:"locationProfileId,omitempty"`
	ThresholdProfileID   *string `json:"thresholdProfileId,omitempty"`
	AlertProfileID       *string `json:"alertProfileId,omitempty"`
	GroupID              *string `json:"groupId,omitempty"`
	Tags                 *string `json:"tags,omitempty"`
	EnvironmentID        *string `json:"environmentId,omitempty"`
}

// dnsMonitorUpdateBody mirrors the PATCH DTO; pointers omit unchanged fields.
type dnsMonitorUpdateBody = dnsMonitorCreateBody

type dnsMonitorResponse struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Status               string  `json:"status"`
	Hostname             string  `json:"hostname"`
	RecordType           string  `json:"recordType"`
	Nameserver           *string `json:"nameserver"`
	ExpectedValues       *string `json:"expectedValues"`
	AlertOnChange        bool    `json:"alertOnChange"`
	RequireDnssec        bool    `json:"requireDnssec"`
	CompareNameservers   bool    `json:"compareNameservers"`
	LastResolvedValues   *string `json:"lastResolvedValues"`
	CheckIntervalSeconds int64   `json:"checkIntervalSeconds"`
	TimeoutSeconds       int64   `json:"timeoutSeconds"`
	LocationProfileID    *string `json:"locationProfileId"`
	ThresholdProfileID   *string `json:"thresholdProfileId"`
	AlertProfileID       *string `json:"alertProfileId"`
	SlaProfileID         *string `json:"slaProfileId"`
	GroupID              *string `json:"groupId"`
	Tags                 *string `json:"tags"`
	EnvironmentID        *string `json:"environmentId,omitempty"`
	CreatedAt            string  `json:"createdAt"`
}

func (r *dnsMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_monitor"
}

func (r *dnsMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *dnsMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A DNS monitor that resolves a hostname's records and alerts when the resolved " +
			"values change or stop matching the expected set.",
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
				MarkdownDescription: "Hostname to resolve.",
				Required:            true,
			},
			"record_type": schema.StringAttribute{
				MarkdownDescription: "DNS record type to query (e.g. `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `CAA`). Defaults to `A`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("A"),
			},
			"nameserver": schema.StringAttribute{
				MarkdownDescription: "Specific nameserver to query against; defaults to the resolver's default.",
				Optional:            true,
			},
			"expected_values": schema.StringAttribute{
				MarkdownDescription: "Comma-separated expected record values; a mismatch fails the check.",
				Optional:            true,
			},
			"alert_on_change": schema.BoolAttribute{
				MarkdownDescription: "Raise an alert whenever the resolved values change. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"require_dnssec": schema.BoolAttribute{
				MarkdownDescription: "Warn unless the answer is DNSSEC-validated (AD flag), asked of `nameserver` " +
					"or public validating resolvers. Hosted locations only. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"compare_nameservers": schema.BoolAttribute{
				MarkdownDescription: "Query every authoritative nameserver of the zone directly and warn when " +
					"their answers disagree. Hosted locations only. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"last_resolved_values": schema.StringAttribute{
				MarkdownDescription: "Most recently resolved values (read-only).",
				Computed:            true,
			},
			"check_interval_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds between checks. Defaults to `300`. Plans cap the minimum.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(300),
			},
			"timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Resolution timeout in seconds. Defaults to `15`.",
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

func (m *dnsMonitorModel) toCreateBody(ctx context.Context, diags *diagSink) *dnsMonitorCreateBody {
	return &dnsMonitorCreateBody{
		Name:                 m.Name.ValueString(),
		Hostname:             m.Hostname.ValueString(),
		RecordType:           strPtr(m.RecordType),
		Nameserver:           strPtr(m.Nameserver),
		ExpectedValues:       strPtr(m.ExpectedValues),
		AlertOnChange:        boolPtr(m.AlertOnChange),
		RequireDnssec:        boolPtr(m.RequireDnssec),
		CompareNameservers:   boolPtr(m.CompareNameservers),
		CheckIntervalSeconds: int64Ptr(m.CheckIntervalSeconds),
		TimeoutSeconds:       int64Ptr(m.TimeoutSeconds),
		LocationProfileID:    strPtr(m.LocationProfileID),
		ThresholdProfileID:   strPtr(m.ThresholdProfileID),
		AlertProfileID:       strPtr(m.AlertProfileID),
		GroupID:              strPtr(m.GroupID),
		Tags:                 csvPtrFromSet(ctx, m.Tags, diags.d),
		EnvironmentID:        strPtr(m.EnvironmentID),
	}
}

// applyResponse copies API response fields into the model.
func (m *dnsMonitorModel) applyResponse(ctx context.Context, resp *dnsMonitorResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Hostname = types.StringValue(resp.Hostname)
	m.RecordType = types.StringValue(resp.RecordType)
	m.Nameserver = strFromPtr(resp.Nameserver)
	m.ExpectedValues = strFromPtr(resp.ExpectedValues)
	m.AlertOnChange = types.BoolValue(resp.AlertOnChange)
	m.RequireDnssec = types.BoolValue(resp.RequireDnssec)
	m.CompareNameservers = types.BoolValue(resp.CompareNameservers)
	m.LastResolvedValues = strFromPtr(resp.LastResolvedValues)
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

func (r *dnsMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsMonitorModel
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
	path := r.client.Tenantf("/monitoring", "/monitors/dns")
	if err := r.client.PostManaged(ctx, path, body, &created, monitorAddress("dns")); err != nil {
		resp.Diagnostics.AddError("Error creating DNS monitor", err.Error())
		return
	}

	// Re-read to populate all computed fields from the canonical record.
	var got dnsMonitorResponse
	if slaIsSet(plan.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, monitorAddress("dns"), created.ID, plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error attaching SLA profile to DNS monitor", err.Error())
			return
		}
	}
	getPath := r.client.Tenantf("/monitoring", "/monitors/dns/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading DNS monitor after create", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got dnsMonitorResponse
	getPath := r.client.Tenantf("/monitoring", "/monitors/dns/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading DNS monitor", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsMonitorModel
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

	path := r.client.Tenantf("/monitoring", "/monitors/dns/%s", plan.ID.ValueString())
	if err := r.client.PatchManaged(ctx, path, body, nil, monitorAddress("dns")); err != nil {
		resp.Diagnostics.AddError("Error updating DNS monitor", err.Error())
		return
	}

	var priorState dnsMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
	if !resp.Diagnostics.HasError() && !plan.SlaProfileID.Equal(priorState.SlaProfileID) {
		if err := applySlaProfile(ctx, r.client, monitorAddress("dns"), plan.ID.ValueString(), plan.SlaProfileID); err != nil {
			resp.Diagnostics.AddError("Error updating SLA profile on DNS monitor", err.Error())
			return
		}
	}

	var got dnsMonitorResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading DNS monitor after update", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitors/dns/%s", state.ID.ValueString())
	if err := r.client.DeleteManaged(ctx, path, monitorAddress("dns")); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting DNS monitor", err.Error())
	}
}

func (r *dnsMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importManagedMonitor(ctx, r.client, "dns", req, resp)
}
