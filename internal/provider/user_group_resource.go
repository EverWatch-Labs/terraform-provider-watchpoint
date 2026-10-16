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

// A user group (GroupsController). This resource manages the group core only -
// name, description and ssoGroupId. Member, role and child/parent links are
// separate sub-endpoints and are intentionally not modelled here. Create/Update
// return only {id,name,success}, so both re-read the full record via GET.

var (
	_ resource.Resource                = (*userGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*userGroupResource)(nil)
	_ resource.ResourceWithImportState = (*userGroupResource)(nil)
)

// NewUserGroupResource is the resource constructor wired into the provider.
func NewUserGroupResource() resource.Resource { return &userGroupResource{} }

type userGroupResource struct {
	client *client.Client
}

// userGroupModel maps schema attributes <-> Terraform state.
type userGroupModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SSOGroupID  types.String `tfsdk:"sso_group_id"`
	IsActive    types.Bool   `tfsdk:"is_active"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the Identity API) ---

type userGroupCreateBody struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SSOGroupID  *string `json:"ssoGroupId,omitempty"`
}

type userGroupResponse struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenantId"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	SSOGroupID  *string `json:"ssoGroupId"`
	IsActive    bool    `json:"isActive"`
	CreatedAt   string  `json:"createdAt"`
}

func (r *userGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_group"
}

func (r *userGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *userGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A user group used for organizing members and (optionally) mapping to an SSO " +
			"group. Membership and role assignments are managed outside this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Group ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Human-readable description of the group.",
				Optional:            true,
			},
			"sso_group_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the SSO group this group maps to, if any.",
				Optional:            true,
			},
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Whether the group is active.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *userGroupModel) toCreateBody() *userGroupCreateBody {
	return &userGroupCreateBody{
		Name:        m.Name.ValueString(),
		Description: strPtr(m.Description),
		SSOGroupID:  strPtr(m.SSOGroupID),
	}
}

// applyResponse copies API response fields into the model.
func (m *userGroupModel) applyResponse(resp *userGroupResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.SSOGroupID = strFromPtr(resp.SSOGroupID)
	m.IsActive = types.BoolValue(resp.IsActive)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
}

func (r *userGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created idNameResponse
	path := r.client.Tenantf("/identity", "/groups")
	if err := r.client.Post(ctx, path, plan.toCreateBody(), &created); err != nil {
		resp.Diagnostics.AddError("Error creating user group", err.Error())
		return
	}

	var got userGroupResponse
	getPath := r.client.Tenantf("/identity", "/groups/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading user group after create", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got userGroupResponse
	getPath := r.client.Tenantf("/identity", "/groups/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading user group", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *userGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/identity", "/groups/%s", plan.ID.ValueString())
	if err := r.client.Put(ctx, path, plan.toCreateBody(), nil); err != nil {
		resp.Diagnostics.AddError("Error updating user group", err.Error())
		return
	}

	var got userGroupResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading user group after update", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/identity", "/groups/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting user group", err.Error())
	}
}

func (r *userGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
