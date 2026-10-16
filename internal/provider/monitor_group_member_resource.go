package provider

import (
	"context"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// monitorGroupMemberResource manages membership of a single monitor in a
// monitor group. There is no update endpoint, so every user-settable attribute
// forces replacement. Its synthetic id is "<groupId>/<monitorId>".
var (
	_ resource.Resource                = (*monitorGroupMemberResource)(nil)
	_ resource.ResourceWithConfigure   = (*monitorGroupMemberResource)(nil)
	_ resource.ResourceWithImportState = (*monitorGroupMemberResource)(nil)
)

// NewMonitorGroupMemberResource is the resource constructor wired into the provider.
func NewMonitorGroupMemberResource() resource.Resource { return &monitorGroupMemberResource{} }

type monitorGroupMemberResource struct {
	client *client.Client
}

// monitorGroupMemberModel maps schema attributes <-> Terraform state.
type monitorGroupMemberModel struct {
	ID          types.String `tfsdk:"id"`
	GroupID     types.String `tfsdk:"group_id"`
	MonitorID   types.String `tfsdk:"monitor_id"`
	MonitorType types.String `tfsdk:"monitor_type"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type monitorGroupMemberBody struct {
	MonitorID   string `json:"monitorId"`
	MonitorType string `json:"monitorType"`
}

// monitorGroupMemberWire mirrors the members[] projection on the tenant group
// listing (GET /groups), used to confirm a member still exists on Read.
type monitorGroupMemberWire struct {
	MonitorID   string `json:"monitorId"`
	MonitorType string `json:"monitorType"`
}

type monitorGroupWithMembers struct {
	ID      string                   `json:"id"`
	Members []monitorGroupMemberWire `json:"members"`
}

func (r *monitorGroupMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_group_member"
}

func (r *monitorGroupMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *monitorGroupMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Membership of a single monitor in a monitor group. This resource has no update " +
			"endpoint; any change recreates the membership. The id is `<group_id>/<monitor_id>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Synthetic membership ID, `<group_id>/<monitor_id>`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "Monitor group to add the monitor to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"monitor_id": schema.StringAttribute{
				MarkdownDescription: "Monitor to add to the group.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"monitor_type": schema.StringAttribute{
				MarkdownDescription: "Monitor type (e.g. `HttpApi`, `Ssl`, `Dns`, `Tcp`, `Ping`, `Synthetic`).",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

// memberExists fetches the group and reports whether monitorId is a member.
func (r *monitorGroupMemberResource) memberExists(ctx context.Context, groupID, monitorID string) (bool, error) {
	var groups []monitorGroupWithMembers
	path := r.client.Tenantf("/monitoring", "/groups")
	if err := r.client.Get(ctx, path, nil, &groups); err != nil {
		return false, err
	}
	for _, g := range groups {
		if g.ID != groupID {
			continue
		}
		for _, m := range g.Members {
			if m.MonitorID == monitorID {
				return true, nil
			}
		}
		return false, nil
	}
	return false, nil
}

func (r *monitorGroupMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorGroupMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &monitorGroupMemberBody{
		MonitorID:   plan.MonitorID.ValueString(),
		MonitorType: plan.MonitorType.ValueString(),
	}
	path := r.client.Tenantf("/monitoring", "/groups/%s/members", plan.GroupID.ValueString())
	if err := r.client.Post(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error adding monitor group member", err.Error())
		return
	}

	plan.ID = types.StringValue(plan.GroupID.ValueString() + "/" + plan.MonitorID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorGroupMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorGroupMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	exists, err := r.memberExists(ctx, state.GroupID.ValueString(), state.MonitorID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading monitor group member", err.Error())
		return
	}
	if !exists {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = types.StringValue(state.GroupID.ValueString() + "/" + state.MonitorID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never invoked: every attribute forces replacement. It exists only
// to satisfy the resource interface and copies plan -> state.
func (r *monitorGroupMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan monitorGroupMemberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorGroupMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorGroupMemberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/groups/%s/members/%s",
		state.GroupID.ValueString(), state.MonitorID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error removing monitor group member", err.Error())
	}
}

func (r *monitorGroupMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the form \"<group_id>/<monitor_id>\", got: "+req.ID,
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("monitor_id"), parts[1])...)
}
