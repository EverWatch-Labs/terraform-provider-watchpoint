package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Mail server monitor (SMTP / IMAP / POP3). The password is write-only: the API
// encrypts it and returns only has_password, so state keeps the configured value.

var (
	_ resource.Resource                = (*mailMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*mailMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*mailMonitorResource)(nil)
)

// NewMailMonitorResource is the resource constructor wired into the provider.
func NewMailMonitorResource() resource.Resource { return &mailMonitorResource{} }

type mailMonitorResource struct {
	client *client.Client
}

type mailMonitorModel struct {
	monitorCommonModel
	Host                   types.String `tfsdk:"host"`
	Port                   types.Int64  `tfsdk:"port"`
	Protocol               types.String `tfsdk:"protocol"`
	TLSMode                types.String `tfsdk:"tls_mode"`
	Username               types.String `tfsdk:"username"`
	Password               types.String `tfsdk:"password"`
	HasPassword            types.Bool   `tfsdk:"has_password"`
	ExpectedBannerContains types.String `tfsdk:"expected_banner_contains"`
	ValidateCertificate    types.Bool   `tfsdk:"validate_certificate"`
	MaxResponseTimeMs      types.Int64  `tfsdk:"max_response_time_ms"`
}

type mailMonitorBody struct {
	monitorCommonBody
	Host                   string  `json:"host"`
	Port                   *int64  `json:"port,omitempty"`
	Protocol               *string `json:"protocol,omitempty"`
	TLSMode                *string `json:"tlsMode,omitempty"`
	Username               *string `json:"username,omitempty"`
	Password               *string `json:"password,omitempty"`
	ExpectedBannerContains *string `json:"expectedBannerContains,omitempty"`
	ValidateCertificate    *bool   `json:"validateCertificate,omitempty"`
	MaxResponseTimeMs      *int64  `json:"maxResponseTimeMs,omitempty"`
}

type mailMonitorResponse struct {
	monitorCommonResponse
	Host                   string  `json:"host"`
	Port                   int64   `json:"port"`
	Protocol               string  `json:"protocol"`
	TLSMode                string  `json:"tlsMode"`
	Username               *string `json:"username"`
	HasPassword            bool    `json:"hasPassword"`
	ExpectedBannerContains *string `json:"expectedBannerContains"`
	ValidateCertificate    bool    `json:"validateCertificate"`
	MaxResponseTimeMs      *int64  `json:"maxResponseTimeMs"`
}

func (r *mailMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_monitor"
}

func (r *mailMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *mailMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := monitorCommonAttributes(300, 30)
	attrs["host"] = schema.StringAttribute{
		MarkdownDescription: "Mail server host name or IP.",
		Required:            true,
	}
	attrs["port"] = schema.Int64Attribute{
		MarkdownDescription: "Port. Defaults to `25`.",
		Optional:            true,
		Computed:            true,
		Default:             int64default.StaticInt64(25),
		Validators:          []validator.Int64{int64validator.Between(1, 65535)},
	}
	attrs["protocol"] = schema.StringAttribute{
		MarkdownDescription: "`Smtp`, `Imap` or `Pop3`. Defaults to `Smtp`.",
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString("Smtp"),
		Validators:          []validator.String{stringvalidator.OneOf("Smtp", "Imap", "Pop3")},
	}
	attrs["tls_mode"] = schema.StringAttribute{
		MarkdownDescription: "`None`, `StartTls` or `Implicit`. Defaults to `StartTls`. A login is refused " +
			"over a plaintext connection.",
		Optional:   true,
		Computed:   true,
		Default:    stringdefault.StaticString("StartTls"),
		Validators: []validator.String{stringvalidator.OneOf("None", "StartTls", "Implicit")},
	}
	attrs["username"] = schema.StringAttribute{
		MarkdownDescription: "Optional login user (SMTP AUTH PLAIN, IMAP LOGIN or POP3 USER/PASS).",
		Optional:            true,
	}
	attrs["password"] = schema.StringAttribute{
		MarkdownDescription: "Login password. Write-only on the API (encrypted at rest, never returned), so " +
			"a change made outside Terraform is not detected. A `{{vault.KEY}}` reference is supported.",
		Optional:  true,
		Sensitive: true,
	}
	attrs["has_password"] = schema.BoolAttribute{
		MarkdownDescription: "Whether a password is stored.",
		Computed:            true,
	}
	attrs["expected_banner_contains"] = schema.StringAttribute{
		MarkdownDescription: "Substring the server greeting must contain.",
		Optional:            true,
	}
	attrs["validate_certificate"] = schema.BoolAttribute{
		MarkdownDescription: "Fail on an invalid TLS certificate. Defaults to `true`.",
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(true),
	}
	attrs["max_response_time_ms"] = schema.Int64Attribute{
		MarkdownDescription: "Fail when the exchange takes longer than this many milliseconds.",
		Optional:            true,
		Validators:          []validator.Int64{int64validator.Between(1, 600000)},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A mail server monitor: connects over SMTP, IMAP or POP3 (plain, STARTTLS or " +
			"implicit TLS), optionally logs in, and asserts on the banner and response time. Requires the " +
			"`mail_monitors` feature. Hosted locations only.",
		Attributes: attrs,
	}
}

func (m *mailMonitorModel) toBody(ctx context.Context, update bool, ds *diagSink) *mailMonitorBody {
	b := &mailMonitorBody{
		monitorCommonBody:      m.body(ctx, update, ds),
		Host:                   m.Host.ValueString(),
		Port:                   int64Ptr(m.Port),
		Protocol:               strPtr(m.Protocol),
		TLSMode:                strPtr(m.TLSMode),
		Username:               strPtr(m.Username),
		Password:               strPtr(m.Password),
		ExpectedBannerContains: strPtr(m.ExpectedBannerContains),
		ValidateCertificate:    boolPtr(m.ValidateCertificate),
		MaxResponseTimeMs:      int64Ptr(m.MaxResponseTimeMs),
	}
	if update {
		// "" clears the optional strings and 0 clears the threshold; null would keep them.
		b.Username = strPtrClearing(m.Username)
		b.Password = strPtrClearing(m.Password)
		b.ExpectedBannerContains = strPtrClearing(m.ExpectedBannerContains)
		if b.MaxResponseTimeMs == nil {
			zero := int64(0)
			b.MaxResponseTimeMs = &zero
		}
	}
	return b
}

func (m *mailMonitorModel) applyResponse(ctx context.Context, in *mailMonitorResponse, ds *diagSink) {
	m.apply(ctx, &in.monitorCommonResponse, ds)
	m.Host = types.StringValue(in.Host)
	m.Port = types.Int64Value(in.Port)
	m.Protocol = types.StringValue(in.Protocol)
	m.TLSMode = types.StringValue(in.TLSMode)
	m.Username = strFromPtr(in.Username)
	m.HasPassword = types.BoolValue(in.HasPassword)
	if !in.HasPassword {
		m.Password = types.StringNull()
	}
	m.ExpectedBannerContains = strFromPtr(in.ExpectedBannerContains)
	m.ValidateCertificate = types.BoolValue(in.ValidateCertificate)
	m.MaxResponseTimeMs = int64FromPtr(in.MaxResponseTimeMs)
}

func (r *mailMonitorResource) api() monitorAPI { return monitorAPI{client: r.client, kind: "mail"} }

func (r *mailMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mailMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	var got mailMonitorResponse
	if err := r.api().create(ctx, plan.toBody(ctx, false, ds), plan.SlaProfileID, &got); err != nil {
		resp.Diagnostics.AddError("Error creating mail monitor", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mailMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got mailMonitorResponse
	found, err := r.api().read(ctx, state.ID.ValueString(), &got)
	if err != nil {
		resp.Diagnostics.AddError("Error reading mail monitor", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(ctx, &got, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mailMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state mailMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	var got mailMonitorResponse
	if err := r.api().update(ctx, state.ID.ValueString(), plan.toBody(ctx, true, ds),
		plan.SlaProfileID, state.SlaProfileID, &got); err != nil {
		resp.Diagnostics.AddError("Error updating mail monitor", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mailMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api().delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting mail monitor", err.Error())
	}
}

func (r *mailMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importManagedMonitor(ctx, r.client, "mail", req, resp)
}
