package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A service account (Identity, /service-accounts): a non-human principal that
// holds one role and its own API keys. Keys are watchpoint_api_key resources with
// service_account_id set. There is no GET by id, so Read lists and selects.

var (
	_ resource.Resource                = (*serviceAccountResource)(nil)
	_ resource.ResourceWithConfigure   = (*serviceAccountResource)(nil)
	_ resource.ResourceWithImportState = (*serviceAccountResource)(nil)
)

// NewServiceAccountResource is the resource constructor wired into the provider.
func NewServiceAccountResource() resource.Resource { return &serviceAccountResource{} }

type serviceAccountResource struct {
	client *client.Client
}

type serviceAccountModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	RoleID         types.String `tfsdk:"role_id"`
	IsActive       types.Bool   `tfsdk:"is_active"`
	RoleName       types.String `tfsdk:"role_name"`
	ActiveKeyCount types.Int64  `tfsdk:"active_key_count"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

type serviceAccountBody struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	RoleID      string  `json:"roleId"`
	IsActive    *bool   `json:"isActive,omitempty"`
}

type serviceAccountResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	RoleID         *string `json:"roleId"`
	RoleName       *string `json:"roleName"`
	IsActive       bool    `json:"isActive"`
	CreatedAt      string  `json:"createdAt"`
	ActiveKeyCount int64   `json:"activeKeyCount"`
}

func (r *serviceAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (r *serviceAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *serviceAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A service account: a non-human principal that holds one role and its own API " +
			"keys. It never counts as a seat and cannot sign in. A key minted for it (a `watchpoint_api_key` " +
			"with `service_account_id`) can never do more than the role allows, whatever scopes it asks for. " +
			"Deactivating the account stops its keys at once; deleting it revokes them. Reads need " +
			"`settings.manage`, writes `apikeys.manage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Service account ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Unique name within the tenant (128 characters max).",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Optional:            true,
			},
			"role_id": schema.StringAttribute{
				MarkdownDescription: "Role in this tenant that caps everything the account's keys can do.",
				Required:            true,
			},
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Whether the account's keys work. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"role_name": schema.StringAttribute{
				MarkdownDescription: "Name of the role, for display.",
				Computed:            true,
			},
			"active_key_count": schema.Int64Attribute{
				MarkdownDescription: "Number of live keys held by the account.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (m *serviceAccountModel) toBody() *serviceAccountBody {
	return &serviceAccountBody{
		Name:        m.Name.ValueString(),
		Description: strPtrClearing(m.Description),
		RoleID:      m.RoleID.ValueString(),
		IsActive:    boolPtr(m.IsActive),
	}
}

func (m *serviceAccountModel) applyResponse(in *serviceAccountResponse) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.Description = strFromPtr(in.Description)
	m.RoleID = strFromPtr(in.RoleID)
	m.RoleName = strFromPtr(in.RoleName)
	m.IsActive = types.BoolValue(in.IsActive)
	m.ActiveKeyCount = types.Int64Value(in.ActiveKeyCount)
	m.CreatedAt = types.StringValue(in.CreatedAt)
}

func (r *serviceAccountResource) path(suffix string) string {
	return r.client.Tenantf("/identity", "/service-accounts%s", suffix)
}

func (r *serviceAccountResource) find(ctx context.Context, id string) (*serviceAccountResponse, error) {
	var rows []serviceAccountResponse
	if err := r.client.Get(ctx, r.path(""), nil, &rows); err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i], nil
		}
	}
	return nil, nil
}

func (r *serviceAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var created serviceAccountResponse
	if err := r.client.Post(ctx, r.path(""), plan.toBody(), &created); err != nil {
		resp.Diagnostics.AddError("Error creating service account", err.Error())
		return
	}
	if !plan.IsActive.ValueBool() {
		if err := r.client.Patch(ctx, r.path("/"+created.ID), plan.toBody(), nil); err != nil {
			resp.Diagnostics.AddError("Error deactivating new service account", err.Error())
			return
		}
	}
	got, err := r.find(ctx, created.ID)
	if err != nil || got == nil {
		resp.Diagnostics.AddError("Error reading service account after create", errText(err, "not found"))
		return
	}
	plan.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.find(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading service account", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serviceAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := plan.ID.ValueString()
	if err := r.client.Patch(ctx, r.path("/"+id), plan.toBody(), nil); err != nil {
		resp.Diagnostics.AddError("Error updating service account", err.Error())
		return
	}
	got, err := r.find(ctx, id)
	if err != nil || got == nil {
		resp.Diagnostics.AddError("Error reading service account after update", errText(err, "not found"))
		return
	}
	plan.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serviceAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path("/"+state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting service account", err.Error())
	}
}

func (r *serviceAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
