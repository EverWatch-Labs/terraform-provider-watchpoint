package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// cost_allocation_settings is the tenant-wide showback convention the mappings
// created by watchpoint_cost_allocation_tag are reported under: whether the
// shared part of the bill (the plan fee, and anything else the usage records do
// not explain) is spread across the tag values in proportion to their metered
// spend, or left in its own explicit untagged row.
//
// Its OWN resource rather than an attribute on cost_allocation_tag, because the
// setting belongs to the tenant and not to a mapping. On the tag resource each
// of a hundred mappings would declare the same single tenant-wide value, so the
// convention actually applied would be whichever mapping Terraform happened to
// write last, and removing any one mapping would leave it ambiguous whether the
// convention went with it.
//
// This is a per-tenant SINGLETON: the row always exists (the API answers with
// the default when nothing is stored), so there is no create and no destroy.
// Create and Update are the same PUT, and Delete only drops the resource from
// state, deliberately leaving the stored convention in place. `false` is a real
// convention ("report shared cost in its own row") rather than the absence of
// one, so there is nothing for a destroy to unset it back to.
//
// The showback report separately accepts ?allocateSharedCost= as a what-if
// override for a single call, which does NOT write the stored value, so Read
// only ever observes what this resource manages.
//
// The id is the tenant id, which is what the provider is already scoped to.

var (
	_ resource.Resource                = (*costAllocationSettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*costAllocationSettingsResource)(nil)
	_ resource.ResourceWithImportState = (*costAllocationSettingsResource)(nil)
)

// NewCostAllocationSettingsResource is the resource constructor wired into the provider.
func NewCostAllocationSettingsResource() resource.Resource { return &costAllocationSettingsResource{} }

type costAllocationSettingsResource struct {
	client *client.Client
}

// costAllocationSettingsModel maps schema attributes <-> Terraform state.
type costAllocationSettingsModel struct {
	ID                 types.String `tfsdk:"id"`
	AllocateSharedCost types.Bool   `tfsdk:"allocate_shared_cost"`
}

// --- Wire DTOs (camelCase JSON matching the billing API) ---

// costAllocationSettingsBody is the PUT body. allocateSharedCost is a plain
// bool and not a pointer on purpose: the API rejects an absent field with a 400
// rather than treating it as "leave unchanged", so there is no omitempty call
// to make here.
type costAllocationSettingsBody struct {
	AllocateSharedCost bool `json:"allocateSharedCost"`
}

type costAllocationSettingsResponse struct {
	AllocateSharedCost bool `json:"allocateSharedCost"`
}

func (r *costAllocationSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cost_allocation_settings"
}

func (r *costAllocationSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *costAllocationSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The tenant's stored cost-allocation showback convention, which every " +
			"showback report and CSV export is produced under. A per-tenant singleton: declare it at " +
			"most once, alongside the `watchpoint_cost_allocation_tag` mappings it governs. Destroying " +
			"it leaves the stored convention in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tenant ID, which is what this per-tenant setting is keyed on.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"allocate_shared_cost": schema.BoolAttribute{
				MarkdownDescription: "Whether shared cost is apportioned across tag values. `true` spreads " +
					"the subscription fee and any spend the usage records do not explain across each tag " +
					"value in proportion to its metered spend, so every value carries a share of the " +
					"platform bill. `false` reports that spend in its own explicit row and leaves the " +
					"tagged values showing metered cost only. Required: the API rejects an omitted value, " +
					"so there is no \"leave as it is\" setting to express.",
				Required: true,
			},
		},
	}
}

func (m *costAllocationSettingsModel) toBody() *costAllocationSettingsBody {
	return &costAllocationSettingsBody{AllocateSharedCost: m.AllocateSharedCost.ValueBool()}
}

// applyResponse folds the API's answer into state. The server value is taken
// verbatim rather than kept from config, so a convention someone flipped in the
// portal reads back as real drift the next plan proposes putting right.
func (m *costAllocationSettingsModel) applyResponse(tenantID string, resp *costAllocationSettingsResponse) {
	m.ID = types.StringValue(tenantID)
	m.AllocateSharedCost = types.BoolValue(resp.AllocateSharedCost)
}

func (r *costAllocationSettingsResource) settingsPath() string {
	return r.client.Tenantf("/billing", "/cost-tags/settings")
}

func (r *costAllocationSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan costAllocationSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated costAllocationSettingsResponse
	if err := r.client.Put(ctx, r.settingsPath(), plan.toBody(), &updated); err != nil {
		resp.Diagnostics.AddError("Error creating cost allocation settings", err.Error())
		return
	}

	plan.applyResponse(r.client.TenantID(), &updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *costAllocationSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state costAllocationSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current costAllocationSettingsResponse
	if err := r.client.Get(ctx, r.settingsPath(), nil, &current); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading cost allocation settings", err.Error())
		return
	}

	state.applyResponse(r.client.TenantID(), &current)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *costAllocationSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan costAllocationSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated costAllocationSettingsResponse
	if err := r.client.Put(ctx, r.settingsPath(), plan.toBody(), &updated); err != nil {
		resp.Diagnostics.AddError("Error updating cost allocation settings", err.Error())
		return
	}

	plan.applyResponse(r.client.TenantID(), &updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the resource. See the file comment: the setting is a
// tenant-wide convention with no unset state, so there is nothing to call.
func (r *costAllocationSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState accepts the tenant id. The value is cosmetic: Read re-derives the
// id from the client's own tenant scope, so any id imports the one row.
func (r *costAllocationSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
