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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UDP monitor: send one datagram, optionally assert on the reply.

var (
	_ resource.Resource                = (*udpMonitorResource)(nil)
	_ resource.ResourceWithConfigure   = (*udpMonitorResource)(nil)
	_ resource.ResourceWithImportState = (*udpMonitorResource)(nil)
)

// NewUDPMonitorResource is the resource constructor wired into the provider.
func NewUDPMonitorResource() resource.Resource { return &udpMonitorResource{} }

type udpMonitorResource struct {
	client *client.Client
}

type udpMonitorModel struct {
	monitorCommonModel
	Host           types.String `tfsdk:"host"`
	Port           types.Int64  `tfsdk:"port"`
	Payload        types.String `tfsdk:"payload"`
	PayloadIsHex   types.Bool   `tfsdk:"payload_is_hex"`
	ExpectContains types.String `tfsdk:"expect_contains"`
	ExpectRegex    types.String `tfsdk:"expect_regex"`
}

type udpMonitorBody struct {
	monitorCommonBody
	Host           string  `json:"host"`
	Port           *int64  `json:"port,omitempty"`
	Payload        *string `json:"payload,omitempty"`
	PayloadIsHex   *bool   `json:"payloadIsHex,omitempty"`
	ExpectContains *string `json:"expectContains,omitempty"`
	ExpectRegex    *string `json:"expectRegex,omitempty"`
}

type udpMonitorResponse struct {
	monitorCommonResponse
	Host           string  `json:"host"`
	Port           int64   `json:"port"`
	Payload        *string `json:"payload"`
	PayloadIsHex   bool    `json:"payloadIsHex"`
	ExpectContains *string `json:"expectContains"`
	ExpectRegex    *string `json:"expectRegex"`
}

func (r *udpMonitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_udp_monitor"
}

func (r *udpMonitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *udpMonitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := monitorCommonAttributes(300, 30)
	attrs["host"] = schema.StringAttribute{
		MarkdownDescription: "Target host name or IP.",
		Required:            true,
	}
	attrs["port"] = schema.Int64Attribute{
		MarkdownDescription: "UDP port. Defaults to `53`.",
		Optional:            true,
		Computed:            true,
		Default:             int64default.StaticInt64(53),
		Validators:          []validator.Int64{int64validator.Between(1, 65535)},
	}
	attrs["payload"] = schema.StringAttribute{
		MarkdownDescription: "Datagram to send (4096 characters max). Text, or hex when `payload_is_hex` is set.",
		Optional:            true,
		Validators:          []validator.String{stringvalidator.LengthAtMost(4096)},
	}
	attrs["payload_is_hex"] = schema.BoolAttribute{
		MarkdownDescription: "Treat `payload` as hex bytes. Defaults to `false`.",
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(false),
	}
	attrs["expect_contains"] = schema.StringAttribute{
		MarkdownDescription: "Substring the reply must contain. With no expectation a silent server passes, " +
			"since UDP has no handshake to fail.",
		Optional:   true,
		Validators: []validator.String{stringvalidator.LengthAtMost(1024)},
	}
	attrs["expect_regex"] = schema.StringAttribute{
		MarkdownDescription: "Regular expression the reply must match.",
		Optional:            true,
		Validators:          []validator.String{stringvalidator.LengthAtMost(1024)},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A UDP monitor: sends one datagram and optionally asserts on the reply; an ICMP " +
			"port-unreachable fails the check. Requires the `udp_monitors` feature. Hosted locations only.",
		Attributes: attrs,
	}
}

func (m *udpMonitorModel) toBody(ctx context.Context, update bool, ds *diagSink) *udpMonitorBody {
	b := &udpMonitorBody{
		monitorCommonBody: m.body(ctx, update, ds),
		Host:              m.Host.ValueString(),
		Port:              int64Ptr(m.Port),
		Payload:           strPtr(m.Payload),
		PayloadIsHex:      boolPtr(m.PayloadIsHex),
		ExpectContains:    strPtr(m.ExpectContains),
		ExpectRegex:       strPtr(m.ExpectRegex),
	}
	if update {
		b.Payload = strPtrClearing(m.Payload)
		b.ExpectContains = strPtrClearing(m.ExpectContains)
		b.ExpectRegex = strPtrClearing(m.ExpectRegex)
	}
	return b
}

func (m *udpMonitorModel) applyResponse(ctx context.Context, in *udpMonitorResponse, ds *diagSink) {
	m.apply(ctx, &in.monitorCommonResponse, ds)
	m.Host = types.StringValue(in.Host)
	m.Port = types.Int64Value(in.Port)
	m.Payload = strFromPtr(in.Payload)
	m.PayloadIsHex = types.BoolValue(in.PayloadIsHex)
	m.ExpectContains = strFromPtr(in.ExpectContains)
	m.ExpectRegex = strFromPtr(in.ExpectRegex)
}

func (r *udpMonitorResource) api() monitorAPI { return monitorAPI{client: r.client, kind: "udp"} }

func (r *udpMonitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan udpMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	var got udpMonitorResponse
	if err := r.api().create(ctx, plan.toBody(ctx, false, ds), plan.SlaProfileID, &got); err != nil {
		resp.Diagnostics.AddError("Error creating UDP monitor", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *udpMonitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state udpMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got udpMonitorResponse
	found, err := r.api().read(ctx, state.ID.ValueString(), &got)
	if err != nil {
		resp.Diagnostics.AddError("Error reading UDP monitor", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(ctx, &got, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *udpMonitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state udpMonitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	var got udpMonitorResponse
	if err := r.api().update(ctx, state.ID.ValueString(), plan.toBody(ctx, true, ds),
		plan.SlaProfileID, state.SlaProfileID, &got); err != nil {
		resp.Diagnostics.AddError("Error updating UDP monitor", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *udpMonitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state udpMonitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api().delete(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting UDP monitor", err.Error())
	}
}

func (r *udpMonitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
