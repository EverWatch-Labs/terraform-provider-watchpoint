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

// deploymentResource manages a deployment marker correlated with check
// results. There is no full update endpoint exposed by this provider, so every
// user-settable attribute forces replacement.
var (
	_ resource.Resource                = (*deploymentResource)(nil)
	_ resource.ResourceWithConfigure   = (*deploymentResource)(nil)
	_ resource.ResourceWithImportState = (*deploymentResource)(nil)
)

// NewDeploymentResource is the resource constructor wired into the provider.
func NewDeploymentResource() resource.Resource { return &deploymentResource{} }

type deploymentResource struct {
	client *client.Client
}

// deploymentModel maps schema attributes <-> Terraform state.
type deploymentModel struct {
	ID              types.String `tfsdk:"id"`
	Version         types.String `tfsdk:"version"`
	Service         types.String `tfsdk:"service"`
	Description     types.String `tfsdk:"description"`
	Environment     types.String `tfsdk:"environment"`
	CommitHash      types.String `tfsdk:"commit_hash"`
	CommitURL       types.String `tfsdk:"commit_url"`
	PipelineURL     types.String `tfsdk:"pipeline_url"`
	Tags            types.String `tfsdk:"tags"`
	DeployedAt      types.String `tfsdk:"deployed_at"`
	EndedAt         types.String `tfsdk:"ended_at"`
	MonitorIDs      types.Set    `tfsdk:"monitor_ids"`
	CreatedByUserID types.String `tfsdk:"created_by_user_id"`
	CreatedByName   types.String `tfsdk:"created_by_name"`
	CreatedAt       types.String `tfsdk:"created_at"`
	IsInProgress    types.Bool   `tfsdk:"is_in_progress"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type deploymentCreateBody struct {
	Version     string   `json:"version"`
	Service     *string  `json:"service,omitempty"`
	Description *string  `json:"description,omitempty"`
	Environment *string  `json:"environment,omitempty"`
	CommitHash  *string  `json:"commitHash,omitempty"`
	CommitURL   *string  `json:"commitUrl,omitempty"`
	PipelineURL *string  `json:"pipelineUrl,omitempty"`
	Tags        *string  `json:"tags,omitempty"`
	DeployedAt  *string  `json:"deployedAt,omitempty"`
	EndedAt     *string  `json:"endedAt,omitempty"`
	MonitorIDs  []string `json:"monitorIds,omitempty"`
}

// deploymentResponse mirrors the MapDeployment projection. monitorIds is an
// array on the response (the request takes it as an array too).
type deploymentResponse struct {
	ID              string   `json:"id"`
	Version         string   `json:"version"`
	Service         *string  `json:"service"`
	Description     *string  `json:"description"`
	Environment     *string  `json:"environment"`
	CommitHash      *string  `json:"commitHash"`
	CommitURL       *string  `json:"commitUrl"`
	PipelineURL     *string  `json:"pipelineUrl"`
	Tags            *string  `json:"tags"`
	DeployedAt      string   `json:"deployedAt"`
	EndedAt         *string  `json:"endedAt"`
	MonitorIDs      []string `json:"monitorIds"`
	CreatedByUserID *string  `json:"createdByUserId"`
	CreatedByName   *string  `json:"createdByName"`
	CreatedAt       string   `json:"createdAt"`
	IsInProgress    bool     `json:"isInProgress"`
}

func (r *deploymentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment"
}

func (r *deploymentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *deploymentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A deployment marker records a release for correlation with check results. This " +
			"resource has no update endpoint; any change recreates the marker.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Deployment marker ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Release version, e.g. `v2.3.1` or a commit short SHA.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"service": schema.StringAttribute{
				MarkdownDescription: "Optional service name used to scope the marker.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Release notes / summary.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"environment": schema.StringAttribute{
				MarkdownDescription: "Deployment environment. Defaults to `production` server-side.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"commit_hash": schema.StringAttribute{
				MarkdownDescription: "Commit hash for the release.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"commit_url": schema.StringAttribute{
				MarkdownDescription: "Link to the commit in GitHub/GitLab.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"pipeline_url": schema.StringAttribute{
				MarkdownDescription: "Link to the CI/CD pipeline run.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"tags": schema.StringAttribute{
				MarkdownDescription: "Comma-separated tags.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"deployed_at": schema.StringAttribute{
				MarkdownDescription: "When the deployment window begins (RFC 3339). Defaults to now server-side.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ended_at": schema.StringAttribute{
				MarkdownDescription: "When the deployment window ends (RFC 3339). Null while in progress.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"monitor_ids": schema.SetAttribute{
				MarkdownDescription: "Monitor IDs this deployment explicitly targets. Empty = applies tenant-wide. Unordered.",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"created_by_user_id": schema.StringAttribute{
				MarkdownDescription: "User ID that recorded the deployment.",
				Computed:            true,
			},
			"created_by_name": schema.StringAttribute{
				MarkdownDescription: "Name of the user that recorded the deployment.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the marker was recorded (RFC 3339).",
				Computed:            true,
			},
			"is_in_progress": schema.BoolAttribute{
				MarkdownDescription: "Whether the deployment window is still open (no `ended_at`).",
				Computed:            true,
			},
		},
	}
}

func (m *deploymentModel) applyResponse(ctx context.Context, resp *deploymentResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Version = types.StringValue(resp.Version)
	m.Service = strFromPtr(resp.Service)
	m.Description = strFromPtr(resp.Description)
	m.Environment = strFromPtr(resp.Environment)
	m.CommitHash = strFromPtr(resp.CommitHash)
	m.CommitURL = strFromPtr(resp.CommitURL)
	m.PipelineURL = strFromPtr(resp.PipelineURL)
	m.Tags = strFromPtr(resp.Tags)
	m.DeployedAt = types.StringValue(resp.DeployedAt)
	m.EndedAt = strFromPtr(resp.EndedAt)
	m.MonitorIDs = setFromStringSlice(ctx, normalizeMonitorIDs(resp.MonitorIDs), diags.d)
	m.CreatedByUserID = strFromPtr(resp.CreatedByUserID)
	m.CreatedByName = strFromPtr(resp.CreatedByName)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.IsInProgress = types.BoolValue(resp.IsInProgress)
}

// normalizeMonitorIDs maps the API's empty-array "tenant-wide" marker back to a
// null list so a config that omits monitor_ids stays consistent with state.
func normalizeMonitorIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func (r *deploymentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan deploymentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := &deploymentCreateBody{
		Version:     plan.Version.ValueString(),
		Service:     strPtr(plan.Service),
		Description: strPtr(plan.Description),
		Environment: strPtr(plan.Environment),
		CommitHash:  strPtr(plan.CommitHash),
		CommitURL:   strPtr(plan.CommitURL),
		PipelineURL: strPtr(plan.PipelineURL),
		Tags:        strPtr(plan.Tags),
		DeployedAt:  strPtr(plan.DeployedAt),
		EndedAt:     strPtr(plan.EndedAt),
		MonitorIDs:  stringSliceFromSet(ctx, plan.MonitorIDs, &resp.Diagnostics),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var created deploymentResponse
	path := r.client.Tenantf("/monitoring", "/deployments")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating deployment", err.Error())
		return
	}
	plan.applyResponse(ctx, &created, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *deploymentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state deploymentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got deploymentResponse
	getPath := r.client.Tenantf("/monitoring", "/deployments/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading deployment", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never invoked: every attribute forces replacement. It exists only
// to satisfy the resource interface and copies plan -> state.
func (r *deploymentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan deploymentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *deploymentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state deploymentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/deployments/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting deployment", err.Error())
	}
}

func (r *deploymentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
