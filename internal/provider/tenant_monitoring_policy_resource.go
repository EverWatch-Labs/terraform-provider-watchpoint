package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Tenant-wide monitor change policy (Monitoring, /monitoring-policy): the
// Terraform-managed lock and change approvals. Each half needs its own plan
// feature, so only the attributes present in config are sent.

var (
	_ resource.Resource                = (*tenantMonitoringPolicyResource)(nil)
	_ resource.ResourceWithConfigure   = (*tenantMonitoringPolicyResource)(nil)
	_ resource.ResourceWithImportState = (*tenantMonitoringPolicyResource)(nil)
)

// NewTenantMonitoringPolicyResource is the resource constructor wired into the provider.
func NewTenantMonitoringPolicyResource() resource.Resource { return &tenantMonitoringPolicyResource{} }

type tenantMonitoringPolicyResource struct {
	client *client.Client
}

type tenantMonitoringPolicyModel struct {
	ID                     types.String `tfsdk:"id"`
	LockTerraformManaged   types.Bool   `tfsdk:"lock_terraform_managed"`
	ApprovalEnvironmentIDs types.Set    `tfsdk:"approval_environment_ids"`
	ApprovalMinApprovers   types.Int64  `tfsdk:"approval_min_approvers"`
	ApprovalAllowSelf      types.Bool   `tfsdk:"approval_allow_self"`
	LockAvailable          types.Bool   `tfsdk:"lock_available"`
	ApprovalsAvailable     types.Bool   `tfsdk:"approvals_available"`
}

type monitoringPolicyBody struct {
	LockTerraformManaged   *bool     `json:"lockTerraformManaged,omitempty"`
	ApprovalEnvironmentIDs *[]string `json:"approvalEnvironmentIds,omitempty"` // non-nil [] turns approvals off
	ApprovalMinApprovers   *int64    `json:"approvalMinApprovers,omitempty"`
	ApprovalAllowSelf      *bool     `json:"approvalAllowSelf,omitempty"`
}

type monitoringPolicyResponse struct {
	LockTerraformManaged   bool     `json:"lockTerraformManaged"`
	LockAvailable          bool     `json:"lockAvailable"`
	ApprovalEnvironmentIDs []string `json:"approvalEnvironmentIds"`
	ApprovalMinApprovers   int64    `json:"approvalMinApprovers"`
	ApprovalAllowSelf      bool     `json:"approvalAllowSelf"`
	ApprovalsAvailable     bool     `json:"approvalsAvailable"`
}

func (r *tenantMonitoringPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_monitoring_policy"
}

func (r *tenantMonitoringPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *tenantMonitoringPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Tenant-wide monitor change policy: lock Terraform-managed monitors against edits made " +
			"outside Terraform, and require approval for monitor changes in chosen environments. One per tenant. " +
			"The lock needs the Terraform managed lock feature and the approval settings need the change approvals " +
			"feature; only the attributes you set are sent, so a tenant with one feature can still manage it. " +
			"While approvals are on for an environment, Terraform writes to its monitors fail with " +
			"`approval_required` like any other caller. Destroying the resource turns the lock and approvals off. " +
			"Requires `settings.manage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The tenant ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"lock_terraform_managed": schema.BoolAttribute{
				MarkdownDescription: "Refuse edits, deletes and reverts of Terraform-managed monitors made outside Terraform.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"approval_environment_ids": schema.SetAttribute{
				MarkdownDescription: "Environments (`watchpoint_environment` ids) whose monitor changes need approval. " +
					"An empty set turns approvals off.",
				Optional:      true,
				Computed:      true,
				ElementType:   types.StringType,
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"approval_min_approvers": schema.Int64Attribute{
				MarkdownDescription: "Approvals a change needs, 1 to 5.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64validator.Between(1, 5)},
			},
			"approval_allow_self": schema.BoolAttribute{
				MarkdownDescription: "Whether the requester may approve their own change.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"lock_available": schema.BoolAttribute{
				MarkdownDescription: "Whether the tenant's plan includes the Terraform managed lock.",
				Computed:            true,
			},
			"approvals_available": schema.BoolAttribute{
				MarkdownDescription: "Whether the tenant's plan includes change approvals.",
				Computed:            true,
			},
		},
	}
}

func (r *tenantMonitoringPolicyResource) path() string {
	return r.client.Tenantf("/monitoring", "/monitoring-policy")
}

func (m *tenantMonitoringPolicyModel) applyResponse(ctx context.Context, tenantID string, in *monitoringPolicyResponse, ds *diagSink) {
	m.ID = types.StringValue(tenantID)
	m.LockTerraformManaged = types.BoolValue(in.LockTerraformManaged)
	m.ApprovalEnvironmentIDs = setFromStringSlice(ctx, emptyIfNil(in.ApprovalEnvironmentIDs), ds.d)
	m.ApprovalMinApprovers = types.Int64Value(in.ApprovalMinApprovers)
	m.ApprovalAllowSelf = types.BoolValue(in.ApprovalAllowSelf)
	m.LockAvailable = types.BoolValue(in.LockAvailable)
	m.ApprovalsAvailable = types.BoolValue(in.ApprovalsAvailable)
}

// write PUTs only what the config sets; with nothing set it just reads.
func (r *tenantMonitoringPolicyResource) write(ctx context.Context, config *tenantMonitoringPolicyModel, ds *diagSink) (*monitoringPolicyResponse, error) {
	body := monitoringPolicyBody{
		LockTerraformManaged: boolPtr(config.LockTerraformManaged),
		ApprovalMinApprovers: int64Ptr(config.ApprovalMinApprovers),
		ApprovalAllowSelf:    boolPtr(config.ApprovalAllowSelf),
	}
	if !config.ApprovalEnvironmentIDs.IsNull() && !config.ApprovalEnvironmentIDs.IsUnknown() {
		ids := emptyIfNil(stringSliceFromSet(ctx, config.ApprovalEnvironmentIDs, ds.d))
		body.ApprovalEnvironmentIDs = &ids
	}
	var out monitoringPolicyResponse
	if body.LockTerraformManaged == nil && body.ApprovalEnvironmentIDs == nil && body.ApprovalMinApprovers == nil && body.ApprovalAllowSelf == nil {
		return &out, r.client.Get(ctx, r.path(), nil, &out)
	}
	return &out, r.client.Put(ctx, r.path(), &body, &out)
}

func (r *tenantMonitoringPolicyResource) apply(ctx context.Context, cfg tfsdk.Config, plan *tenantMonitoringPolicyModel, ds *diagSink) {
	var config tenantMonitoringPolicyModel
	ds.d.Append(cfg.Get(ctx, &config)...)
	if ds.d.HasError() {
		return
	}
	out, err := r.write(ctx, &config, ds)
	if err != nil {
		ds.d.AddError("Error writing monitoring policy", err.Error())
		return
	}
	plan.applyResponse(ctx, r.client.TenantID(), out, ds)
}

func (r *tenantMonitoringPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tenantMonitoringPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	ds := &diagSink{d: &resp.Diagnostics}
	r.apply(ctx, req.Config, &plan, ds)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *tenantMonitoringPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tenantMonitoringPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out monitoringPolicyResponse
	if err := r.client.Get(ctx, r.path(), nil, &out); err != nil {
		resp.Diagnostics.AddError("Error reading monitoring policy", err.Error())
		return
	}
	state.applyResponse(ctx, r.client.TenantID(), &out, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *tenantMonitoringPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan tenantMonitoringPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	ds := &diagSink{d: &resp.Diagnostics}
	r.apply(ctx, req.Config, &plan, ds)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

// Delete turns off whichever half is on and available.
func (r *tenantMonitoringPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var cur monitoringPolicyResponse
	if err := r.client.Get(ctx, r.path(), nil, &cur); err != nil {
		resp.Diagnostics.AddError("Error reading monitoring policy", err.Error())
		return
	}
	var body monitoringPolicyBody
	if cur.LockAvailable && cur.LockTerraformManaged {
		off := false
		body.LockTerraformManaged = &off
	}
	if cur.ApprovalsAvailable && len(cur.ApprovalEnvironmentIDs) > 0 {
		body.ApprovalEnvironmentIDs = &[]string{}
	}
	if body.LockTerraformManaged == nil && body.ApprovalEnvironmentIDs == nil {
		return
	}
	if err := r.client.Put(ctx, r.path(), &body, nil); err != nil {
		resp.Diagnostics.AddError("Error resetting monitoring policy", err.Error())
	}
}

// ImportState accepts any id (conventionally the tenant ID); Read fills the rest.
func (r *tenantMonitoringPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
