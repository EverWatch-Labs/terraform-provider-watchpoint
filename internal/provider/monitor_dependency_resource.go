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

// monitorDependencyResource manages a single directed dependency edge between
// monitors (or a monitor and a group). There is no update endpoint, so every
// user-settable attribute forces replacement.
var (
	_ resource.Resource                = (*monitorDependencyResource)(nil)
	_ resource.ResourceWithConfigure   = (*monitorDependencyResource)(nil)
	_ resource.ResourceWithImportState = (*monitorDependencyResource)(nil)
)

// NewMonitorDependencyResource is the resource constructor wired into the provider.
func NewMonitorDependencyResource() resource.Resource { return &monitorDependencyResource{} }

type monitorDependencyResource struct {
	client *client.Client
}

// monitorDependencyModel maps schema attributes <-> Terraform state.
type monitorDependencyModel struct {
	ID                 types.String `tfsdk:"id"`
	DependentMonitorID types.String `tfsdk:"dependent_monitor_id"`
	DependsOnMonitorID types.String `tfsdk:"depends_on_monitor_id"`
	DependsOnGroupID   types.String `tfsdk:"depends_on_group_id"`
	Description        types.String `tfsdk:"description"`
	CreatedAt          types.String `tfsdk:"created_at"`
	CreatedByUserID    types.String `tfsdk:"created_by_user_id"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type monitorDependencyCreateBody struct {
	DependentMonitorID string  `json:"dependentMonitorId"`
	DependsOnMonitorID *string `json:"dependsOnMonitorId,omitempty"`
	DependsOnGroupID   *string `json:"dependsOnGroupId,omitempty"`
	Description        *string `json:"description,omitempty"`
}

type monitorDependencyResponse struct {
	ID                 string  `json:"id"`
	DependentMonitorID string  `json:"dependentMonitorId"`
	DependsOnMonitorID *string `json:"dependsOnMonitorId"`
	DependsOnGroupID   *string `json:"dependsOnGroupId"`
	Description        *string `json:"description"`
	CreatedAt          string  `json:"createdAt"`
	CreatedByUserID    *string `json:"createdByUserId"`
}

// monitorDependencyList is the {items:[...]} envelope returned by the list
// endpoint (the only Read surface for an individual edge).
type monitorDependencyList struct {
	Items []monitorDependencyResponse `json:"items"`
}

func (r *monitorDependencyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_dependency"
}

func (r *monitorDependencyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *monitorDependencyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A directed dependency edge: the dependent monitor's alerts are dampened when its " +
			"upstream is unhealthy. Exactly one of `depends_on_monitor_id` or `depends_on_group_id` must be set. " +
			"This resource has no update endpoint; any change recreates the edge.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Dependency edge ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"dependent_monitor_id": schema.StringAttribute{
				MarkdownDescription: "The monitor whose alerts are dampened when the upstream is unhealthy.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"depends_on_monitor_id": schema.StringAttribute{
				MarkdownDescription: "Upstream monitor this edge depends on. Mutually exclusive with `depends_on_group_id`.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"depends_on_group_id": schema.StringAttribute{
				MarkdownDescription: "Upstream monitor group this edge depends on; expands to every member at walk time. " +
					"Mutually exclusive with `depends_on_monitor_id`.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Free-form reason the edge exists.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
			"created_by_user_id": schema.StringAttribute{
				MarkdownDescription: "User ID that created the edge.",
				Computed:            true,
			},
		},
	}
}

func (m *monitorDependencyModel) applyResponse(resp *monitorDependencyResponse) {
	m.ID = types.StringValue(resp.ID)
	m.DependentMonitorID = types.StringValue(resp.DependentMonitorID)
	m.DependsOnMonitorID = strFromPtr(resp.DependsOnMonitorID)
	m.DependsOnGroupID = strFromPtr(resp.DependsOnGroupID)
	m.Description = strFromPtr(resp.Description)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.CreatedByUserID = strFromPtr(resp.CreatedByUserID)
}

// findDependencyByID lists all tenant edges and returns the one matching id.
// The controller only exposes a list endpoint for reading edges. Returns a nil
// pointer with no error when the edge is absent.
func (r *monitorDependencyResource) findDependencyByID(ctx context.Context, id string) (*monitorDependencyResponse, error) {
	var list monitorDependencyList
	path := r.client.Tenantf("/monitoring", "/monitor-dependencies")
	if err := r.client.Get(ctx, path, nil, &list); err != nil {
		return nil, err
	}
	for i := range list.Items {
		if list.Items[i].ID == id {
			return &list.Items[i], nil
		}
	}
	return nil, nil
}

func (r *monitorDependencyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorDependencyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &monitorDependencyCreateBody{
		DependentMonitorID: plan.DependentMonitorID.ValueString(),
		DependsOnMonitorID: strPtr(plan.DependsOnMonitorID),
		DependsOnGroupID:   strPtr(plan.DependsOnGroupID),
		Description:        strPtr(plan.Description),
	}

	var created monitorDependencyResponse
	path := r.client.Tenantf("/monitoring", "/monitor-dependencies")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating monitor dependency", err.Error())
		return
	}

	plan.applyResponse(&created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorDependencyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorDependencyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.findDependencyByID(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading monitor dependency", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never invoked: every attribute forces replacement. It exists only
// to satisfy the resource interface and copies plan -> state.
func (r *monitorDependencyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan monitorDependencyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorDependencyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorDependencyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/monitor-dependencies/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting monitor dependency", err.Error())
	}
}

func (r *monitorDependencyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
