package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Per-monitor settings that are written only through the shared bulk endpoint
// (PATCH /monitors/bulk) rather than the per-type DTOs: cross-region down
// confirmation and the owning team. One resource covers every monitor type.
//
// ponytail: the per-type GET responses do not return these fields yet, so Read
// keeps state as written and a change made in the portal is not detected. When
// the monitor GETs expose confirmationPolicy / quorumMinRegions / ownerGroupId,
// read them back here.

var (
	_ resource.Resource                = (*monitorPolicyResource)(nil)
	_ resource.ResourceWithConfigure   = (*monitorPolicyResource)(nil)
	_ resource.ResourceWithImportState = (*monitorPolicyResource)(nil)
)

// NewMonitorPolicyResource is the resource constructor wired into the provider.
func NewMonitorPolicyResource() resource.Resource { return &monitorPolicyResource{} }

type monitorPolicyResource struct {
	client *client.Client
}

type monitorPolicyModel struct {
	ID                 types.String `tfsdk:"id"`
	MonitorID          types.String `tfsdk:"monitor_id"`
	ConfirmationPolicy types.String `tfsdk:"confirmation_policy"`
	QuorumMinRegions   types.Int64  `tfsdk:"quorum_min_regions"`
	OwnerGroupID       types.String `tfsdk:"owner_group_id"`
}

type monitorBulkBody struct {
	MonitorIDs         []string `json:"monitorIds"`
	ConfirmationPolicy *string  `json:"confirmationPolicy,omitempty"`
	QuorumMinRegions   *int64   `json:"quorumMinRegions,omitempty"`
	OwnerGroupID       *string  `json:"ownerGroupId,omitempty"`
}

type monitorBulkResponse struct {
	Results []struct {
		ID     string  `json:"id"`
		Status string  `json:"status"`
		Error  *string `json:"error"`
	} `json:"results"`
}

func (r *monitorPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_policy"
}

func (r *monitorPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *monitorPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Cross-region down confirmation and the owning team for one monitor of any type. " +
			"These settings are written through the platform's bulk monitor endpoint and are not returned by " +
			"the monitor reads, so a change made in the portal is not detected as drift. Destroying the " +
			"resource resets confirmation to `None` and clears the owner.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Same as `monitor_id`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"monitor_id": schema.StringAttribute{
				MarkdownDescription: "ID of the monitor (any type) the settings apply to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"confirmation_policy": schema.StringAttribute{
				MarkdownDescription: "`None` (every failing result counts), `ConfirmFromAnotherRegion` (a failure " +
					"is re-run from a different region and counts only if that fails too) or `Quorum` (down only " +
					"while at least `quorum_min_regions` regions are failing). Defaults to `None`. Anything that " +
					"cannot confirm, such as a private location, counts the failure as before.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString("None"),
				Validators: []validator.String{stringvalidator.OneOf("None", "ConfirmFromAnotherRegion", "Quorum")},
			},
			"quorum_min_regions": schema.Int64Attribute{
				MarkdownDescription: "Failing regions needed for `Quorum`, between `2` and `20`. Capped at the " +
					"number of regions the check actually runs from.",
				Optional:   true,
				Validators: []validator.Int64{int64validator.Between(2, 20)},
			},
			"owner_group_id": schema.StringAttribute{
				MarkdownDescription: "Owning team (a `watchpoint_user_group` id). A filter label; it grants no access.",
				Optional:            true,
			},
		},
	}
}

func (m *monitorPolicyModel) toBody() *monitorBulkBody {
	return &monitorBulkBody{
		MonitorIDs:         []string{m.MonitorID.ValueString()},
		ConfirmationPolicy: strPtr(m.ConfirmationPolicy),
		QuorumMinRegions:   int64Ptr(m.QuorumMinRegions),
		OwnerGroupID:       strPtrClearing(m.OwnerGroupID),
	}
}

// write sends the bulk PATCH and turns a per-monitor "skipped"/"failed" result,
// which arrives inside a 200, into an error.
func (r *monitorPolicyResource) write(ctx context.Context, body *monitorBulkBody) error {
	var out monitorBulkResponse
	if err := r.client.Patch(ctx, r.client.Tenantf("/monitoring", "/monitors/bulk"), body, &out); err != nil {
		return err
	}
	for _, res := range out.Results {
		if res.Status != "updated" {
			return fmt.Errorf("monitor %s was %s: %s", res.ID, res.Status, deref(res.Error))
		}
	}
	return nil
}

func (r *monitorPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, plan.toBody()); err != nil {
		resp.Diagnostics.AddError("Error setting monitor policy", err.Error())
		return
	}
	plan.ID = plan.MonitorID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read keeps state: the fields are write-only on the monitor API today.
func (r *monitorPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if state.MonitorID.IsNull() {
		state.MonitorID = state.ID // import
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *monitorPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan monitorPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, plan.toBody()); err != nil {
		resp.Diagnostics.AddError("Error updating monitor policy", err.Error())
		return
	}
	plan.ID = plan.MonitorID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	none, empty := "None", ""
	err := r.write(ctx, &monitorBulkBody{
		MonitorIDs: []string{state.MonitorID.ValueString()}, ConfirmationPolicy: &none, OwnerGroupID: &empty,
	})
	// A monitor that is already gone takes its settings with it.
	if err != nil && !client.IsNotFound(err) && !isBulkNotFound(err) {
		resp.Diagnostics.AddError("Error resetting monitor policy", err.Error())
	}
}

func isBulkNotFound(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not found")
}

func (r *monitorPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
