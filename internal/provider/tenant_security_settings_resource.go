package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Tenant-wide sign-in security (Identity): MFA enforcement, session timeout and
// the tenant IP allowlist. A per-tenant singleton over two endpoints: the tenant
// PATCH carries MFA and the timeout, and the focused /ip-allowlist PATCH carries
// the ranges because it is the one that validates them as CIDRs.

var (
	_ resource.Resource                = (*tenantSecuritySettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*tenantSecuritySettingsResource)(nil)
	_ resource.ResourceWithImportState = (*tenantSecuritySettingsResource)(nil)
)

// NewTenantSecuritySettingsResource is the resource constructor wired into the provider.
func NewTenantSecuritySettingsResource() resource.Resource { return &tenantSecuritySettingsResource{} }

type tenantSecuritySettingsResource struct {
	client *client.Client
}

type tenantSecuritySettingsModel struct {
	ID                    types.String `tfsdk:"id"`
	EnforceMfa            types.Bool   `tfsdk:"enforce_mfa"`
	SessionTimeoutMinutes types.Int64  `tfsdk:"session_timeout_minutes"`
	IPAllowlistEnabled    types.Bool   `tfsdk:"ip_allowlist_enabled"`
	AllowedIPRanges       types.Set    `tfsdk:"allowed_ip_ranges"`
}

type tenantSecurityPatchBody struct {
	EnforceMfa            *bool  `json:"enforceMfa,omitempty"`
	SessionTimeoutMinutes *int64 `json:"sessionTimeoutMinutes,omitempty"`
}

type tenantIPAllowlistBody struct {
	IPAllowlistEnabled *bool   `json:"ipAllowlistEnabled,omitempty"`
	AllowedIPRanges    *string `json:"allowedIpRanges"`
}

type tenantSecurityResponse struct {
	EnforceMfa            bool    `json:"enforceMfa"`
	SessionTimeoutMinutes int64   `json:"sessionTimeoutMinutes"`
	IPAllowlistEnabled    bool    `json:"ipAllowlistEnabled"`
	AllowedIPRanges       *string `json:"allowedIpRanges"`
}

func (r *tenantSecuritySettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_security_settings"
}

func (r *tenantSecuritySettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *tenantSecuritySettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Tenant-wide sign-in security: MFA enforcement, the idle session timeout and the " +
			"tenant IP allowlist, which applies to both sign-in and API keys. A per-tenant singleton: declare it " +
			"at most once. Requires `settings.manage`. **Enabling the allowlist without the address Terraform " +
			"runs from locks that run out**; include your CI egress range. Destroying the resource leaves the " +
			"settings as they are rather than silently widening access.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tenant ID, which this setting is keyed on.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enforce_mfa": schema.BoolAttribute{
				MarkdownDescription: "Require every member to enrol in MFA before using the portal. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"session_timeout_minutes": schema.Int64Attribute{
				MarkdownDescription: "Idle auto-logout window in minutes, between `5` and `1440`. Left as the " +
					"server has it when omitted (the platform default is `60`).",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.Int64{int64validator.Between(5, 1440)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"ip_allowlist_enabled": schema.BoolAttribute{
				MarkdownDescription: "Restrict sign-in and API-key use to `allowed_ip_ranges`. Defaults to `false`. " +
					"With no ranges set, an enabled allowlist allows everything.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"allowed_ip_ranges": schema.SetAttribute{
				MarkdownDescription: "IPv4/IPv6 CIDR ranges (a bare address is a single host). Validated by the API.",
				Optional:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (m *tenantSecuritySettingsModel) applyResponse(ctx context.Context, tenantID string, in *tenantSecurityResponse, ds *diagSink) {
	m.ID = types.StringValue(tenantID)
	m.EnforceMfa = types.BoolValue(in.EnforceMfa)
	m.SessionTimeoutMinutes = types.Int64Value(in.SessionTimeoutMinutes)
	m.IPAllowlistEnabled = types.BoolValue(in.IPAllowlistEnabled)
	m.AllowedIPRanges = setFromCSVPtr(ctx, in.AllowedIPRanges, ds.d)
}

func (r *tenantSecuritySettingsResource) apply(ctx context.Context, plan *tenantSecuritySettingsModel, ds *diagSink) bool {
	tenantPath := r.client.Tenantf("/identity", "")
	if err := r.client.Patch(ctx, tenantPath, &tenantSecurityPatchBody{
		EnforceMfa:            boolPtr(plan.EnforceMfa),
		SessionTimeoutMinutes: int64Ptr(plan.SessionTimeoutMinutes),
	}, nil); err != nil {
		ds.d.AddError("Error updating tenant security settings", err.Error())
		return false
	}
	if err := r.client.Patch(ctx, r.client.Tenantf("/identity", "/ip-allowlist"), &tenantIPAllowlistBody{
		IPAllowlistEnabled: boolPtr(plan.IPAllowlistEnabled),
		AllowedIPRanges:    csvPtrClearing(ctx, plan.AllowedIPRanges, ds.d),
	}, nil); err != nil {
		ds.d.AddError("Error updating tenant IP allowlist", err.Error())
		return false
	}
	var got tenantSecurityResponse
	if err := r.client.Get(ctx, tenantPath, nil, &got); err != nil {
		ds.d.AddError("Error reading tenant security settings", err.Error())
		return false
	}
	plan.applyResponse(ctx, r.client.TenantID(), &got, ds)
	return true
}

func (r *tenantSecuritySettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tenantSecuritySettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.apply(ctx, &plan, &diagSink{d: &resp.Diagnostics}) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *tenantSecuritySettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tenantSecuritySettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got tenantSecurityResponse
	if err := r.client.Get(ctx, r.client.Tenantf("/identity", ""), nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading tenant security settings", err.Error())
		return
	}
	state.applyResponse(ctx, r.client.TenantID(), &got, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *tenantSecuritySettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan tenantSecuritySettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.apply(ctx, &plan, &diagSink{d: &resp.Diagnostics}) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the singleton. Turning MFA enforcement or the allowlist
// off on destroy would widen access as a side effect of a refactor.
func (r *tenantSecuritySettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *tenantSecuritySettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
