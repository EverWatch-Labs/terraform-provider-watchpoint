package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A custom tenant role. RolesController exposes PATCH for name/description/
// isActive only - the permission set is managed through separate sub-endpoints
// (.../permissions). Because permissions cannot be changed in place, every
// user-settable attribute on this resource forces replacement and Update is a
// straight plan->state passthrough.

var (
	_ resource.Resource                = (*roleResource)(nil)
	_ resource.ResourceWithConfigure   = (*roleResource)(nil)
	_ resource.ResourceWithImportState = (*roleResource)(nil)
)

// NewRoleResource is the resource constructor wired into the provider.
func NewRoleResource() resource.Resource { return &roleResource{} }

type roleResource struct {
	client *client.Client
}

// roleModel maps schema attributes <-> Terraform state.
type roleModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Permissions     types.Set    `tfsdk:"permissions"`
	IsSystemDefault types.Bool   `tfsdk:"is_system_default"`
	IsActive        types.Bool   `tfsdk:"is_active"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the Identity API) ---

type roleCreateBody struct {
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

type rolePermissionResponse struct {
	ID          string  `json:"id"`
	Permission  string  `json:"permission"`
	Description *string `json:"description"`
}

type roleResponse struct {
	ID              string                   `json:"id"`
	TenantID        string                   `json:"tenantId"`
	Name            string                   `json:"name"`
	Description     *string                  `json:"description"`
	IsSystemDefault bool                     `json:"isSystemDefault"`
	IsActive        bool                     `json:"isActive"`
	CreatedAt       string                   `json:"createdAt"`
	Permissions     []rolePermissionResponse `json:"permissions"`
}

func (r *roleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *roleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *roleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A custom tenant role with a fixed permission set. The API has no in-place " +
			"update for the permission list, so changing any attribute replaces the role.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Role ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Role name (unique within the tenant).",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Human-readable description of the role.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"permissions": schema.SetAttribute{
				MarkdownDescription: "Permission strings granted to the role. Unordered.",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"is_system_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a built-in system-default role.",
				Computed:            true,
			},
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Whether the role is active.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

// applyResponse copies API response fields into the model.
func (m *roleModel) applyResponse(ctx context.Context, resp *roleResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	perms := make([]string, 0, len(resp.Permissions))
	for _, p := range resp.Permissions {
		perms = append(perms, p.Permission)
	}
	if len(perms) == 0 {
		m.Permissions = types.SetNull(types.StringType)
	} else {
		m.Permissions = setFromStringSlice(ctx, perms, diags.d)
	}
	m.IsSystemDefault = types.BoolValue(resp.IsSystemDefault)
	m.IsActive = types.BoolValue(resp.IsActive)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := &roleCreateBody{
		Name:        plan.Name.ValueString(),
		Description: strPtr(plan.Description),
		Permissions: stringSliceFromSet(ctx, plan.Permissions, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var created roleResponse
	path := r.client.Tenantf("/identity", "/roles")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating role", err.Error())
		return
	}

	plan.applyResponse(ctx, &created, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got roleResponse
	getPath := r.client.Tenantf("/identity", "/roles/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading role", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a passthrough: every user-settable attribute forces replacement, so
// the framework only calls Update for computed-only drift.
func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan roleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/identity", "/roles/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting role", err.Error())
	}
}

func (r *roleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
