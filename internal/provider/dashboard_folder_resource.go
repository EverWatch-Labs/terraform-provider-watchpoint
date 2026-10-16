package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A dashboard folder (Analytics, /dashboard-folders). Folders nest one level
// deep. There is no GET by id, so Read lists and selects. Deleting a folder
// moves its dashboards up to its parent and its subfolders to the root; nothing
// is deleted with it.

var (
	_ resource.Resource                = (*dashboardFolderResource)(nil)
	_ resource.ResourceWithConfigure   = (*dashboardFolderResource)(nil)
	_ resource.ResourceWithImportState = (*dashboardFolderResource)(nil)
)

// NewDashboardFolderResource is the resource constructor wired into the provider.
func NewDashboardFolderResource() resource.Resource { return &dashboardFolderResource{} }

type dashboardFolderResource struct {
	client *client.Client
}

type dashboardFolderModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	ParentID  types.String `tfsdk:"parent_id"`
	CreatedAt types.String `tfsdk:"created_at"`
}

// dashboardFolderBody serves POST and PATCH. parentId is always sent: on PATCH
// "" moves the folder to the root (null would leave it nested), on POST "" means
// root too.
type dashboardFolderBody struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parentId"`
}

type dashboardFolderResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ParentID  *string `json:"parentId"`
	CreatedAt string  `json:"createdAt"`
}

func (r *dashboardFolderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard_folder"
}

func (r *dashboardFolderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *dashboardFolderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A folder that groups custom dashboards in the portal. Folders nest one level " +
			"deep. File a dashboard in it with `folder_id` on `watchpoint_dashboard`. Destroying the folder " +
			"keeps its contents: dashboards move up to the parent folder and subfolders move to the root. " +
			"Reads need `dashboards.read`, writes `dashboards.manage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Folder ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Folder name (128 characters max).",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 128)},
			},
			"parent_id": schema.StringAttribute{
				MarkdownDescription: "Parent folder. Omit for a top-level folder. The parent must itself be " +
					"top-level, and a folder that has subfolders cannot be nested.",
				Optional: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (m *dashboardFolderModel) toBody() *dashboardFolderBody {
	return &dashboardFolderBody{Name: m.Name.ValueString(), ParentID: strPtrClearing(m.ParentID)}
}

func (m *dashboardFolderModel) applyResponse(in *dashboardFolderResponse) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.ParentID = strFromPtr(in.ParentID)
	m.CreatedAt = types.StringValue(in.CreatedAt)
}

func (r *dashboardFolderResource) path(suffix string) string {
	return r.client.Tenantf("/analytics", "/dashboard-folders%s", suffix)
}

func (r *dashboardFolderResource) find(ctx context.Context, id string) (*dashboardFolderResponse, error) {
	var rows []dashboardFolderResponse
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

func (r *dashboardFolderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dashboardFolderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var created dashboardFolderResponse
	if err := r.client.Post(ctx, r.path(""), plan.toBody(), &created); err != nil {
		resp.Diagnostics.AddError("Error creating dashboard folder", err.Error())
		return
	}
	plan.applyResponse(&created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dashboardFolderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dashboardFolderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.find(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading dashboard folder", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dashboardFolderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dashboardFolderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got dashboardFolderResponse
	if err := r.client.Patch(ctx, r.path("/"+plan.ID.ValueString()), plan.toBody(), &got); err != nil {
		resp.Diagnostics.AddError("Error updating dashboard folder", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dashboardFolderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dashboardFolderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path("/"+state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting dashboard folder", err.Error())
	}
}

func (r *dashboardFolderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
