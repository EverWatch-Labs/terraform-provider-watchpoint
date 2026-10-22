package provider

import (
	"context"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Domain registration monitor: expiry, registrar and nameserver changes read
// from RDAP (WHOIS fallback). Nothing connects to the domain itself.

var (
	_ resource.Resource                = (*domainMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*domainMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*domainMonitorResource)(nil)
)

// NewDomainMonitorResource is the resource constructor wired into the provider.
func NewDomainMonitorResource() resource.Resource { return &domainMonitorResource{} }

type domainMonitorResource struct {
	client *client.Client
}

type domainMonitorModel struct {
	monitorCommonModel
	Domain                  types.String `tfsdk:"domain"`
	ExpiryWarningDays       types.Int64  `tfsdk:"expiry_warning_days"`
	ExpiryCriticalDays      types.Int64  `tfsdk:"expiry_critical_days"`
	AlertOnRegistrarChange  types.Bool   `tfsdk:"alert_on_registrar_change"`
	AlertOnNameserverChange types.Bool   `tfsdk:"alert_on_nameserver_change"`
	Registrar               types.String `tfsdk:"registrar"`
	Nameservers             types.String `tfsdk:"nameservers"`
	ExpiresAt               types.String `tfsdk:"expires_at"`
}

type domainMonitorBody struct {
	monitorCommonBody
	Domain                  string `json:"domain"`
	ExpiryWarningDays       *int64 `json:"expiryWarningDays,omitempty"`
	ExpiryCriticalDays      *int64 `json:"expiryCriticalDays,omitempty"`
	AlertOnRegistrarChange  *bool  `json:"alertOnRegistrarChange,omitempty"`
	AlertOnNameserverChange *bool  `json:"alertOnNameserverChange,omitempty"`
}

type domainMonitorResponse struct {
	monitorCommonResponse
	Domain                  string  `json:"domain"`
	ExpiryWarningDays       int64   `json:"expiryWarningDays"`
	ExpiryCriticalDays      int64   `json:"expiryCriticalDays"`
	AlertOnRegistrarChange  bool    `json:"alertOnRegistrarChange"`
	AlertOnNameserverChange bool    `json:"alertOnNameserverChange"`
	LastKnownRegistrar      *string `json:"lastKnownRegistrar"`
	LastKnownNameservers    *string `json:"lastKnownNameservers"`
	ExpiresAt               *string `json:"expiresAt"`
}

func (r *domainMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain_monitor"
}

func (r *domainMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *domainMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := monitorCommonAttributes(86400, 30)
	attrs["domain"] = schema.StringAttribute{
		MarkdownDescription: "Registered domain name, for example `example.com`. Internationalised names " +
			"are stored as punycode; a difference only in letter case is not drift.",
		Required:   true,
		Validators: []validator.String{stringvalidator.LengthAtMost(253)},
	}
	attrs["expiry_warning_days"] = schema.Int64Attribute{
		MarkdownDescription: "Warn when the registration expires within this many days. Defaults to `30`.",
		Optional:            true,
		Computed:            true,
		Default:             int64default.StaticInt64(30),
		Validators:          []validator.Int64{int64validator.Between(1, 365)},
	}
	attrs["expiry_critical_days"] = schema.Int64Attribute{
		MarkdownDescription: "Go critical within this many days. Must not exceed `expiry_warning_days`. " +
			"Defaults to `7`.",
		Optional:   true,
		Computed:   true,
		Default:    int64default.StaticInt64(7),
		Validators: []validator.Int64{int64validator.Between(0, 365)},
	}
	attrs["alert_on_registrar_change"] = schema.BoolAttribute{
		MarkdownDescription: "Warn when the registrar changes. Defaults to `true`.",
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(true),
	}
	attrs["alert_on_nameserver_change"] = schema.BoolAttribute{
		MarkdownDescription: "Warn when the delegated nameservers change. Defaults to `true`.",
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(true),
	}
	attrs["registrar"] = schema.StringAttribute{
		MarkdownDescription: "Registrar seen on the last check.",
		Computed:            true,
	}
	attrs["nameservers"] = schema.StringAttribute{
		MarkdownDescription: "Comma-separated nameservers seen on the last check.",
		Computed:            true,
	}
	attrs["expires_at"] = schema.StringAttribute{
		MarkdownDescription: "Registration expiry seen on the last check (RFC 3339).",
		Computed:            true,
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A domain registration monitor: reads expiry, registrar and nameservers from " +
			"the registry (RDAP, WHOIS fallback) and alerts before the domain lapses or when its delegation " +
			"changes. Checks daily by default. Requires the `domain_monitors` feature.",
		Attributes: attrs,
	}
}

func (m *domainMonitorModel) toBody(ctx context.Context, update bool, ds *diagSink) *domainMonitorBody {
	return &domainMonitorBody{
		monitorCommonBody:       m.body(ctx, update, ds),
		Domain:                  m.Domain.ValueString(),
		ExpiryWarningDays:       int64Ptr(m.ExpiryWarningDays),
		ExpiryCriticalDays:      int64Ptr(m.ExpiryCriticalDays),
		AlertOnRegistrarChange:  boolPtr(m.AlertOnRegistrarChange),
		AlertOnNameserverChange: boolPtr(m.AlertOnNameserverChange),
	}
}

func (m *domainMonitorModel) applyResponse(ctx context.Context, in *domainMonitorResponse, ds *diagSink) {
	m.apply(ctx, &in.monitorCommonResponse, ds)
	// The API lower-cases (and punycodes) the name; keep the configured spelling
	// when it is the same domain.
	if m.Domain.IsNull() || m.Domain.IsUnknown() || !strings.EqualFold(m.Domain.ValueString(), in.Domain) {
		m.Domain = types.StringValue(in.Domain)
	}
	m.ExpiryWarningDays = types.Int64Value(in.ExpiryWarningDays)
	m.ExpiryCriticalDays = types.Int64Value(in.ExpiryCriticalDays)
	m.AlertOnRegistrarChange = types.BoolValue(in.AlertOnRegistrarChange)
	m.AlertOnNameserverChange = types.BoolValue(in.AlertOnNameserverChange)
	m.Registrar = strFromPtr(in.LastKnownRegistrar)
	m.Nameservers = strFromPtr(in.LastKnownNameservers)
	m.ExpiresAt = strFromPtr(in.ExpiresAt)
}

func (r *domainMonitorResource) api() monitorAPI { return monitorAPI{client: r.client, kind: "domain"} }

func (r *domainMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	var got domainMonitorResponse
	if err := r.api().create(ctx, plan.toBody(ctx, false, ds), plan.SlaProfileID, &got); err != nil {
		resp.Diagnostics.AddError("Error creating domain monitor", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state domainMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got domainMonitorResponse
	found, err := r.api().read(ctx, state.ID.ValueString(), &got)
	if err != nil {
		resp.Diagnostics.AddError("Error reading domain monitor", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(ctx, &got, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *domainMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state domainMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	var got domainMonitorResponse
	if err := r.api().update(ctx, state.ID.ValueString(), plan.toBody(ctx, true, ds),
		plan.SlaProfileID, state.SlaProfileID, &got); err != nil {
		resp.Diagnostics.AddError("Error updating domain monitor", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state domainMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api().delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting domain monitor", err.Error())
	}
}

func (r *domainMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importManagedMonitor(ctx, r.client, "domain", req, resp)
}
