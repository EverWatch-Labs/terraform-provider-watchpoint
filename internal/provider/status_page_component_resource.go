package provider

import (
	"context"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// status_page_component is one row on a status page: the unit visitors see go
// green or red. A component rolls up the health of the monitors attached to it,
// or mirrors somebody else's status page through
// watchpoint_status_page_external_source.
//
// The Status API owns it (Watchpoint.Status.Api StatusPagesController). There is
// no GET for a single component, so Read pulls the owning page's detail
// projection and picks this component out of its `components` array; a component
// that has gone is removed from state rather than erroring.
//
// status_page_id forces replacement: a component cannot be moved between pages.
//
// monitor_ids and depends_on_component_ids are unordered membership, so both are
// Sets and never Lists. They are written through their own PUT endpoints, which
// replace the whole membership, so an empty set really does clear it.
//
// The monitors endpoint silently drops any monitor ID the tenant does not own
// (it filters through the Analytics health API). A configuration naming a
// foreign monitor therefore fails the apply with an inconsistent-result error
// rather than quietly attaching nothing, which is the louder of the two.

var (
	_ resource.Resource                = (*statusPageComponentResource)(nil)
	_ resource.ResourceWithConfigure   = (*statusPageComponentResource)(nil)
	_ resource.ResourceWithImportState = (*statusPageComponentResource)(nil)
)

// statusPageAggregations are the ComponentAggregation names the API accepts.
var statusPageAggregations = []string{"WorstOf", "AllFailed", "Majority"}

// NewStatusPageComponentResource is the resource constructor wired into the provider.
func NewStatusPageComponentResource() resource.Resource { return &statusPageComponentResource{} }

type statusPageComponentResource struct {
	client *client.Client
}

// statusPageComponentModel maps schema attributes <-> Terraform state.
type statusPageComponentModel struct {
	ID              types.String `tfsdk:"id"`
	StatusPageID    types.String `tfsdk:"status_page_id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Position        types.Int64  `tfsdk:"position"`
	AggregationMode types.String `tfsdk:"aggregation_mode"`
	GroupID         types.String `tfsdk:"group_id"`
	MonitorIDs      types.Set    `tfsdk:"monitor_ids"`
	DependsOn       types.Set    `tfsdk:"depends_on_component_ids"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the status API) ---

// statusPageComponentCreateBody mirrors CreateComponentRequest.
type statusPageComponentCreateBody struct {
	Name            string   `json:"name"`
	Description     *string  `json:"description,omitempty"`
	Position        *int64   `json:"position,omitempty"`
	AggregationMode *string  `json:"aggregationMode,omitempty"`
	MonitorIDs      []string `json:"monitorIds,omitempty"`
}

// statusPageComponentUpdateBody mirrors PatchComponentRequest.
type statusPageComponentUpdateBody struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	Position        *int64  `json:"position,omitempty"`
	AggregationMode *string `json:"aggregationMode,omitempty"`
	// "" ungroups; null would leave the component in its old group.
	GroupID *string `json:"groupId,omitempty"`
}

// statusPageComponentMonitorsBody mirrors ComponentMonitorsRequest. Never nil:
// the API dereferences the list, so a null would be a 500 rather than a clear.
type statusPageComponentMonitorsBody struct {
	MonitorIDs []string `json:"monitorIds"`
}

// statusPageComponentDependenciesBody mirrors ComponentDependenciesRequest.
type statusPageComponentDependenciesBody struct {
	DependsOnComponentIDs []string `json:"dependsOnComponentIds"`
}

// statusPageComponentResponse mirrors MapComponent, which is also the shape the
// page detail projection nests under `components`.
type statusPageComponentResponse struct {
	ID              string   `json:"id"`
	StatusPageID    string   `json:"statusPageId"`
	Name            string   `json:"name"`
	Description     *string  `json:"description"`
	Position        int64    `json:"position"`
	AggregationMode string   `json:"aggregationMode"`
	GroupID         *string  `json:"groupId"`
	MonitorIDs      []string `json:"monitorIds"`
	DependsOn       []string `json:"dependsOn"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

// statusPageComponentsEnvelope is the slice of the page detail projection this
// resource needs; the rest of the page is watchpoint_status_page's business.
type statusPageComponentsEnvelope struct {
	Components []statusPageComponentResponse `json:"components"`
}

func (r *statusPageComponentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_component"
}

func (r *statusPageComponentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *statusPageComponentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One component on a status page. A component rolls up the health of the monitors " +
			"attached to it, and is what a visitor sees listed as operational or degraded. Attach a " +
			"third-party feed instead with `watchpoint_status_page_external_source`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Component ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status_page_id": schema.StringAttribute{
				MarkdownDescription: "ID of the status page this component belongs to. Changing it replaces the component.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Component name shown to visitors, for example `API` or `Dashboard`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 200)},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional short description shown under the component name.",
				Optional:            true,
			},
			"position": schema.Int64Attribute{
				MarkdownDescription: "Sort order on the page, ascending. Defaults to the end of the current list.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"aggregation_mode": schema.StringAttribute{
				MarkdownDescription: "How the attached monitors roll up into one component health: `WorstOf` " +
					"(as bad as the worst monitor), `AllFailed` (degraded only when every monitor is failing, " +
					"for high-availability groups) or `Majority` (degraded once half or more are failing). " +
					"Defaults to `WorstOf`.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.OneOf(statusPageAggregations...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "`watchpoint_status_page_component_group` on the same page to list this " +
					"component under. Omit to leave it ungrouped.",
				Optional: true,
			},
			"monitor_ids": schema.SetAttribute{
				MarkdownDescription: "Monitors whose health this component reports. Unordered. Monitor IDs the " +
					"tenant does not own are rejected by the API and will fail the apply.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"depends_on_component_ids": schema.SetAttribute{
				MarkdownDescription: "Components on the same page this one depends on, used to explain a knock-on " +
					"outage. Unordered. A component cannot depend on itself and the graph may not contain a cycle.",
				Optional:    true,
				ElementType: types.StringType,
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

func (m *statusPageComponentModel) toCreateBody(ctx context.Context, diags *diagSink) *statusPageComponentCreateBody {
	return &statusPageComponentCreateBody{
		Name:            m.Name.ValueString(),
		Description:     strPtr(m.Description),
		Position:        int64Ptr(m.Position),
		AggregationMode: strPtr(m.AggregationMode),
		MonitorIDs:      stringSliceFromSet(ctx, m.MonitorIDs, diags.d),
	}
}

// toUpdateBody sends description as "" rather than omitting it when the
// attribute is absent from configuration, so removing it actually clears the
// stored value instead of silently leaving the old one in place.
func (m *statusPageComponentModel) toUpdateBody() *statusPageComponentUpdateBody {
	return &statusPageComponentUpdateBody{
		Name:            strPtr(m.Name),
		Description:     strPtrClearing(m.Description),
		Position:        int64Ptr(m.Position),
		AggregationMode: strPtr(m.AggregationMode),
		GroupID:         strPtrClearing(m.GroupID),
	}
}

// findStatusPageComponent picks this component out of the page detail listing,
// or returns nil when it is gone (deleted in the portal).
func findStatusPageComponent(components []statusPageComponentResponse, id string) *statusPageComponentResponse {
	for i := range components {
		if components[i].ID == id {
			return &components[i]
		}
	}
	return nil
}

// applyResponse folds a component row into state. An empty membership array maps
// to a null Set, not an empty one: the API returns `[]` for a membership the
// configuration simply omits, and writing that back as a known empty set makes
// every such apply fail with "was null, but now cty.SetVal([])".
func (m *statusPageComponentModel) applyResponse(ctx context.Context, resp *statusPageComponentResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.StatusPageID = types.StringValue(resp.StatusPageID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.Position = types.Int64Value(resp.Position)
	m.AggregationMode = types.StringValue(resp.AggregationMode)
	m.GroupID = strFromPtr(resp.GroupID)
	m.MonitorIDs = setOrNull(ctx, resp.MonitorIDs, diags)
	m.DependsOn = setOrNull(ctx, resp.DependsOn, diags)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
}

// fetch reads the owning page and returns this component, or nil when the page
// no longer lists it.
func (r *statusPageComponentResource) fetch(ctx context.Context, pageID, componentID string) (*statusPageComponentResponse, error) {
	var page statusPageComponentsEnvelope
	path := r.client.Tenantf("/status", "/pages/%s", pageID)
	if err := r.client.Get(ctx, path, nil, &page); err != nil {
		return nil, err
	}
	return findStatusPageComponent(page.Components, componentID), nil
}

// setMembership replaces the component's monitor list and dependency list. Both
// are whole-set PUTs, so an empty slice clears the membership.
func (r *statusPageComponentResource) setMembership(ctx context.Context, m *statusPageComponentModel, monitors, deps []string) error {
	monitorsPath := r.client.Tenantf("/status", "/pages/%s/components/%s/monitors",
		m.StatusPageID.ValueString(), m.ID.ValueString())
	if err := r.client.Put(ctx, monitorsPath, &statusPageComponentMonitorsBody{MonitorIDs: emptyIfNil(monitors)}, nil); err != nil {
		return err
	}
	depsPath := r.client.Tenantf("/status", "/pages/%s/components/%s/dependencies",
		m.StatusPageID.ValueString(), m.ID.ValueString())
	return r.client.Put(ctx, depsPath, &statusPageComponentDependenciesBody{DependsOnComponentIDs: emptyIfNil(deps)}, nil)
}

func (r *statusPageComponentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toCreateBody(ctx, ds)
	deps := stringSliceFromSet(ctx, plan.DependsOn, ds.d)
	if resp.Diagnostics.HasError() {
		return
	}

	var created statusPageComponentResponse
	path := r.client.Tenantf("/status", "/pages/%s/components", plan.StatusPageID.ValueString())
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating status page component", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)

	// Group membership is a component PATCH, not part of the create body.
	if strPtr(plan.GroupID) != nil {
		patchPath := r.client.Tenantf("/status", "/pages/%s/components/%s", plan.StatusPageID.ValueString(), created.ID)
		if err := r.client.Patch(ctx, patchPath, &statusPageComponentUpdateBody{GroupID: strPtr(plan.GroupID)}, nil); err != nil {
			resp.Diagnostics.AddError("Error placing status page component in its group", err.Error())
			return
		}
	}

	// Dependencies are not part of the create body, so they go in a follow-up
	// PUT. Skipped when there are none: a brand-new component has no edges.
	if len(deps) > 0 {
		depsPath := r.client.Tenantf("/status", "/pages/%s/components/%s/dependencies",
			plan.StatusPageID.ValueString(), created.ID)
		if err := r.client.Put(ctx, depsPath, &statusPageComponentDependenciesBody{DependsOnComponentIDs: deps}, nil); err != nil {
			resp.Diagnostics.AddError("Error creating status page component", err.Error())
			return
		}
	}

	got, err := r.fetch(ctx, plan.StatusPageID.ValueString(), created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading status page component after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Error reading status page component after create",
			"The component was created but is not listed on the status page.")
		return
	}

	plan.applyResponse(ctx, got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageComponentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.fetch(ctx, state.StatusPageID.ValueString(), state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading status page component", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageComponentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	monitors := stringSliceFromSet(ctx, plan.MonitorIDs, ds.d)
	deps := stringSliceFromSet(ctx, plan.DependsOn, ds.d)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/status", "/pages/%s/components/%s",
		plan.StatusPageID.ValueString(), plan.ID.ValueString())
	var updated statusPageComponentResponse
	if err := r.client.Patch(ctx, path, plan.toUpdateBody(), &updated); err != nil {
		resp.Diagnostics.AddError("Error updating status page component", err.Error())
		return
	}
	// Membership lives behind its own PUTs. They are sent every time rather than
	// diffed: each replaces the whole set, so re-sending an unchanged one is a
	// no-op server-side and costs one request.
	if err := r.setMembership(ctx, &plan, monitors, deps); err != nil {
		resp.Diagnostics.AddError("Error updating status page component", err.Error())
		return
	}

	got, err := r.fetch(ctx, plan.StatusPageID.ValueString(), plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading status page component after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Error reading status page component after update",
			"The component is no longer listed on the status page.")
		return
	}

	plan.applyResponse(ctx, got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageComponentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/status", "/pages/%s/components/%s",
		state.StatusPageID.ValueString(), state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting status page component", err.Error())
	}
}

// ImportState takes "<status_page_id>/<component_id>": there is no endpoint that
// resolves a component by id alone, so the page has to come with it.
func (r *statusPageComponentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the form \"<status_page_id>/<component_id>\", got: "+req.ID,
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
