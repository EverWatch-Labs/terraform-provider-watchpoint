package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A collapsible heading over components on a status page (Status,
// /pages/{id}/groups). Membership is set on the component (group_id); the
// group's status and uptime bars are derived at render time. There is no GET by
// id: the page detail lists its groups.

var (
	_ resource.Resource                = (*statusPageComponentGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*statusPageComponentGroupResource)(nil)
	_ resource.ResourceWithImportState = (*statusPageComponentGroupResource)(nil)
)

// NewStatusPageComponentGroupResource is the resource constructor wired into the provider.
func NewStatusPageComponentGroupResource() resource.Resource {
	return &statusPageComponentGroupResource{}
}

type statusPageComponentGroupResource struct {
	client *client.Client
}

type statusPageComponentGroupModel struct {
	ID                 types.String `tfsdk:"id"`
	StatusPageID       types.String `tfsdk:"status_page_id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Position           types.Int64  `tfsdk:"position"`
	CollapsedByDefault types.Bool   `tfsdk:"collapsed_by_default"`
}

type componentGroupBody struct {
	Name               string  `json:"name"`
	Description        *string `json:"description"`
	Position           *int64  `json:"position,omitempty"`
	CollapsedByDefault *bool   `json:"collapsedByDefault,omitempty"`
}

type componentGroupResponse struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Description        *string `json:"description"`
	Position           int64   `json:"position"`
	CollapsedByDefault bool    `json:"collapsedByDefault"`
}

func (r *statusPageComponentGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_component_group"
}

func (r *statusPageComponentGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *statusPageComponentGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A collapsible group of components on a status page. Its status is the worst of " +
			"its components and its uptime bars merge theirs. Put a component in it with `group_id` on " +
			"`watchpoint_status_page_component`. Deleting a group leaves its components on the page, ungrouped.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Group ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status_page_id": schema.StringAttribute{
				MarkdownDescription: "Status page the group belongs to. Changing it replaces the group.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Heading shown to visitors.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Optional:            true,
			},
			"position": schema.Int64Attribute{
				MarkdownDescription: "Sort order among the page's groups and ungrouped components. Defaults to " +
					"the end of the list.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"collapsed_by_default": schema.BoolAttribute{
				MarkdownDescription: "Render the group folded until the visitor opens it. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (m *statusPageComponentGroupModel) toBody() *componentGroupBody {
	return &componentGroupBody{
		Name:               m.Name.ValueString(),
		Description:        strPtrClearing(m.Description),
		Position:           int64Ptr(m.Position),
		CollapsedByDefault: boolPtr(m.CollapsedByDefault),
	}
}

func (m *statusPageComponentGroupModel) applyResponse(in *componentGroupResponse) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.Description = strFromPtr(in.Description)
	m.Position = types.Int64Value(in.Position)
	m.CollapsedByDefault = types.BoolValue(in.CollapsedByDefault)
}

func (r *statusPageComponentGroupResource) path(pageID, id string) string {
	if id == "" {
		return r.client.Tenantf("/status", "/pages/%s/groups", pageID)
	}
	return r.client.Tenantf("/status", "/pages/%s/groups/%s", pageID, id)
}

func (r *statusPageComponentGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageComponentGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got componentGroupResponse
	if err := r.client.Post(ctx, r.path(plan.StatusPageID.ValueString(), ""), plan.toBody(), &got); err != nil {
		resp.Diagnostics.AddError("Error creating status page component group", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageComponentGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageComponentGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var page struct {
		Groups []componentGroupResponse `json:"groups"`
	}
	if err := r.client.Get(ctx, r.client.Tenantf("/status", "/pages/%s", state.StatusPageID.ValueString()), nil, &page); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading status page component group", err.Error())
		return
	}
	for i := range page.Groups {
		if page.Groups[i].ID == state.ID.ValueString() {
			state.applyResponse(&page.Groups[i])
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *statusPageComponentGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageComponentGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got componentGroupResponse
	if err := r.client.Patch(ctx, r.path(plan.StatusPageID.ValueString(), plan.ID.ValueString()), plan.toBody(), &got); err != nil {
		resp.Diagnostics.AddError("Error updating status page component group", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageComponentGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageComponentGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path(state.StatusPageID.ValueString(), state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting status page component group", err.Error())
	}
}

func (r *statusPageComponentGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importChild(ctx, req, resp, "status_page_id")
}
