package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// dashboard_resource.go implements the analytics dashboard resource. The
// Analytics API stores a dashboard's widgets as a raw JSON array column
// (LayoutJson) and its display options as a raw JSON column (SettingsJson),
// so this resource models those as opaque JSON string attributes rather than
// structured nested blocks - the DTO has no typed widget fields to mirror.

var (
	_ resource.Resource                = (*dashboardResource)(nil)
	_ resource.ResourceWithConfigure   = (*dashboardResource)(nil)
	_ resource.ResourceWithImportState = (*dashboardResource)(nil)
)

// NewDashboardResource is the resource constructor wired into the provider.
func NewDashboardResource() resource.Resource { return &dashboardResource{} }

type dashboardResource struct {
	client *client.Client
}

// dashboardModel maps schema attributes <-> Terraform state.
type dashboardModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	LayoutJSON   types.String `tfsdk:"layout_json"`
	SettingsJSON types.String `tfsdk:"settings_json"`
	IsDefault    types.Bool   `tfsdk:"is_default"`
	IsPublic     types.Bool   `tfsdk:"is_public"`
	PublicSlug   types.String `tfsdk:"public_slug"`
	FolderID     types.String `tfsdk:"folder_id"`
	OwnerGroupID types.String `tfsdk:"owner_group_id"`
	CreatedAt    types.String `tfsdk:"created_at"`
	UpdatedAt    types.String `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the Analytics API) ---

// dashboardCreateBody mirrors CreateDashboardRequest. IsPublic/PublicSlug are
// not accepted on create by the controller; they are only settable via PATCH.
type dashboardCreateBody struct {
	Name         string  `json:"name"`
	Description  *string `json:"description,omitempty"`
	LayoutJSON   *string `json:"layoutJson,omitempty"`
	SettingsJSON *string `json:"settingsJson,omitempty"`
	IsDefault    *bool   `json:"isDefault,omitempty"`
	FolderID     *string `json:"folderId,omitempty"`
	OwnerGroupID *string `json:"ownerGroupId,omitempty"`
}

// dashboardUpdateBody mirrors UpdateDashboardRequest; pointers omit unchanged
// fields so a PATCH only touches what is set.
type dashboardUpdateBody struct {
	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	LayoutJSON   *string `json:"layoutJson,omitempty"`
	SettingsJSON *string `json:"settingsJson,omitempty"`
	IsDefault    *bool   `json:"isDefault,omitempty"`
	IsPublic     *bool   `json:"isPublic,omitempty"`
	PublicSlug   *string `json:"publicSlug,omitempty"`
	// "" moves the dashboard to the root / clears the owner; omitted keeps them.
	FolderID     *string `json:"folderId,omitempty"`
	OwnerGroupID *string `json:"ownerGroupId,omitempty"`
}

// dashboardResponse mirrors the Dashboard domain entity returned by
// GET/POST/PATCH.
type dashboardResponse struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	IsDefault    bool    `json:"isDefault"`
	IsPublic     bool    `json:"isPublic"`
	PublicSlug   *string `json:"publicSlug"`
	LayoutJSON   string  `json:"layoutJson"`
	SettingsJSON *string `json:"settingsJson"`
	FolderID     *string `json:"folderId"`
	OwnerGroupID *string `json:"ownerGroupId"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

func (r *dashboardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dashboard"
}

func (r *dashboardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *dashboardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An analytics dashboard: a named collection of widgets laid out on a grid, " +
			"optionally exposed as a public status page.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Dashboard ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable dashboard name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description shown on the dashboard workspace card.",
				Optional:            true,
			},
			"layout_json": schema.StringAttribute{
				MarkdownDescription: "JSON-encoded array of widget definitions, each shaped like " +
					"`{ \"type\": ..., \"config\": { ... }, \"grid\": { \"x\": 0, \"y\": 0, \"w\": 6, \"h\": 4 } }`. " +
					"`config` is a free-form per-widget object. Defaults to an empty array (`[]`).",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("[]"),
			},
			"settings_json": schema.StringAttribute{
				MarkdownDescription: "JSON-encoded dashboard-level settings (theme, refresh interval, time range).",
				Optional:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the tenant's default dashboard. Setting it true clears the " +
					"flag on any other default dashboard. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"is_public": schema.BoolAttribute{
				MarkdownDescription: "Whether the dashboard is exposed as a public status page. May be blocked by " +
					"tenant compliance policy. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"public_slug": schema.StringAttribute{
				MarkdownDescription: "URL slug used when the dashboard is public.",
				Optional:            true,
			},
			"folder_id": schema.StringAttribute{
				MarkdownDescription: "`watchpoint_dashboard_folder` the dashboard is filed in. Omit for the root. " +
					"Deleting the folder moves the dashboard up to the folder's parent.",
				Optional: true,
			},
			"owner_group_id": schema.StringAttribute{
				MarkdownDescription: "User group (`watchpoint_user_group`) that owns this dashboard. A filter " +
					"label only; it grants no access.",
				Optional: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "Last-update timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *dashboardModel) toCreateBody() *dashboardCreateBody {
	return &dashboardCreateBody{
		Name:         m.Name.ValueString(),
		Description:  strPtr(m.Description),
		LayoutJSON:   strPtr(m.LayoutJSON),
		SettingsJSON: strPtr(m.SettingsJSON),
		IsDefault:    boolPtr(m.IsDefault),
		FolderID:     strPtr(m.FolderID),
		OwnerGroupID: strPtr(m.OwnerGroupID),
	}
}

func (m *dashboardModel) toUpdateBody() *dashboardUpdateBody {
	return &dashboardUpdateBody{
		Name:         strPtr(m.Name),
		Description:  strPtr(m.Description),
		LayoutJSON:   strPtr(m.LayoutJSON),
		SettingsJSON: strPtr(m.SettingsJSON),
		IsDefault:    boolPtr(m.IsDefault),
		IsPublic:     boolPtr(m.IsPublic),
		PublicSlug:   strPtr(m.PublicSlug),
		FolderID:     strPtrClearing(m.FolderID),
		OwnerGroupID: strPtrClearing(m.OwnerGroupID),
	}
}

// applyResponse copies API response fields into the model.
func (m *dashboardModel) applyResponse(resp *dashboardResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.LayoutJSON = types.StringValue(resp.LayoutJSON)
	m.SettingsJSON = strFromPtr(resp.SettingsJSON)
	m.IsDefault = types.BoolValue(resp.IsDefault)
	m.IsPublic = types.BoolValue(resp.IsPublic)
	m.PublicSlug = strFromPtr(resp.PublicSlug)
	m.FolderID = strFromPtr(resp.FolderID)
	m.OwnerGroupID = strFromPtr(resp.OwnerGroupID)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
}

func (r *dashboardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dashboardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The create endpoint returns the full dashboard entity.
	var created dashboardResponse
	path := r.client.Tenantf("/analytics", "/dashboards")
	if err := r.client.Post(ctx, path, plan.toCreateBody(), &created); err != nil {
		resp.Diagnostics.AddError("Error creating dashboard", err.Error())
		return
	}

	plan.applyResponse(&created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dashboardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dashboardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got dashboardResponse
	getPath := r.client.Tenantf("/analytics", "/dashboards/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading dashboard", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dashboardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dashboardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The patch endpoint returns the full updated dashboard entity.
	var got dashboardResponse
	path := r.client.Tenantf("/analytics", "/dashboards/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, plan.toUpdateBody(), &got); err != nil {
		resp.Diagnostics.AddError("Error updating dashboard", err.Error())
		return
	}

	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dashboardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dashboardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/analytics", "/dashboards/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting dashboard", err.Error())
	}
}

func (r *dashboardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
