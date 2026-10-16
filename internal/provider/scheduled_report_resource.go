package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// scheduled_report_resource.go implements the analytics scheduled report
// resource. A report renders a dashboard (or runs an SGL/audit query) on a
// recurring schedule and emails the result to a set of recipients. The
// Analytics API has no GET-by-id endpoint for reports, so Read lists the
// tenant's reports and finds the one matching the stored id.

var (
	_ resource.Resource                = (*scheduledReportResource)(nil)
	_ resource.ResourceWithConfigure   = (*scheduledReportResource)(nil)
	_ resource.ResourceWithImportState = (*scheduledReportResource)(nil)
)

// NewScheduledReportResource is the resource constructor wired into the provider.
func NewScheduledReportResource() resource.Resource { return &scheduledReportResource{} }

type scheduledReportResource struct {
	client *client.Client
}

// scheduledReportModel maps schema attributes <-> Terraform state.
type scheduledReportModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	DashboardID     types.String `tfsdk:"dashboard_id"`
	Schedule        types.String `tfsdk:"schedule"`
	Recipients      types.Set    `tfsdk:"recipients"`
	TimeRange       types.String `tfsdk:"time_range"`
	SglQuery        types.String `tfsdk:"sgl_query"`
	LastGeneratedAt types.String `tfsdk:"last_generated_at"`
	NextScheduledAt types.String `tfsdk:"next_scheduled_at"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the Analytics API) ---

// scheduledReportCreateBody mirrors CreateReportRequest. Schedule is a
// PascalCase enum string (None/Daily/Weekly/Monthly); RecipientEmails is a
// comma-separated list.
type scheduledReportCreateBody struct {
	Name            string  `json:"name"`
	DashboardID     *string `json:"dashboardId,omitempty"`
	Schedule        *string `json:"schedule,omitempty"`
	RecipientEmails *string `json:"recipientEmails,omitempty"`
	TimeRange       *string `json:"timeRange,omitempty"`
	SglQuery        *string `json:"sglQuery,omitempty"`
}

// scheduledReportUpdateBody mirrors UpdateReportRequest. DashboardId cannot be
// changed via PATCH, so it is omitted here.
type scheduledReportUpdateBody struct {
	Name            *string `json:"name,omitempty"`
	Schedule        *string `json:"schedule,omitempty"`
	RecipientEmails *string `json:"recipientEmails,omitempty"`
	TimeRange       *string `json:"timeRange,omitempty"`
	SglQuery        *string `json:"sglQuery,omitempty"`
}

// scheduledReportResponse mirrors the Report domain entity returned by
// POST/PATCH and the per-row shape returned by the list endpoint.
type scheduledReportResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	DashboardID     *string `json:"dashboardId"`
	Schedule        string  `json:"schedule"`
	RecipientEmails *string `json:"recipientEmails"`
	TimeRange       string  `json:"timeRange"`
	SglQuery        *string `json:"sglQuery"`
	LastGeneratedAt *string `json:"lastGeneratedAt"`
	NextScheduledAt *string `json:"nextScheduledAt"`
	CreatedAt       string  `json:"createdAt"`
}

func (r *scheduledReportResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scheduled_report"
}

func (r *scheduledReportResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *scheduledReportResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A scheduled report that renders a dashboard (or runs an SGL/audit query) on a " +
			"recurring schedule and emails the result to a list of recipients.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Report ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable report name; used as the email subject.",
				Required:            true,
			},
			"dashboard_id": schema.StringAttribute{
				MarkdownDescription: "Dashboard rendered into this report. Leave unset for an SGL/audit CSV export " +
					"report (see `sgl_query`). Cannot be changed after creation.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"schedule": schema.StringAttribute{
				MarkdownDescription: "Delivery cadence: `None`, `Daily`, `Weekly`, or `Monthly`. Defaults to `None` " +
					"(manual-only). Scheduled reports run at 08:00 UTC.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("None"),
			},
			"recipients": schema.SetAttribute{
				MarkdownDescription: "Email addresses the rendered report is sent to. Unordered.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"time_range": schema.StringAttribute{
				MarkdownDescription: "Time window the report covers (e.g. `1h`, `24h`, `7d`, `30d`). Defaults to `24h`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("24h"),
			},
			"sgl_query": schema.StringAttribute{
				MarkdownDescription: "When set, the report is an SGL/audit-log CSV export run over `time_range` " +
					"instead of a dashboard render. Mutually exclusive with `dashboard_id` in practice.",
				Optional: true,
			},
			"last_generated_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the most recent render (RFC 3339), or null if never generated.",
				Computed:            true,
			},
			"next_scheduled_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the next scheduled run (RFC 3339), or null when `schedule` is `None`.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *scheduledReportModel) toCreateBody(ctx context.Context, diags *diagSink) *scheduledReportCreateBody {
	return &scheduledReportCreateBody{
		Name:            m.Name.ValueString(),
		DashboardID:     strPtr(m.DashboardID),
		Schedule:        strPtr(m.Schedule),
		RecipientEmails: csvPtrFromSet(ctx, m.Recipients, diags.d),
		TimeRange:       strPtr(m.TimeRange),
		SglQuery:        strPtr(m.SglQuery),
	}
}

func (m *scheduledReportModel) toUpdateBody(ctx context.Context, diags *diagSink) *scheduledReportUpdateBody {
	return &scheduledReportUpdateBody{
		Name:            strPtr(m.Name),
		Schedule:        strPtr(m.Schedule),
		RecipientEmails: csvPtrFromSet(ctx, m.Recipients, diags.d),
		TimeRange:       strPtr(m.TimeRange),
		SglQuery:        strPtr(m.SglQuery),
	}
}

// applyResponse copies API response fields into the model.
func (m *scheduledReportModel) applyResponse(ctx context.Context, resp *scheduledReportResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.DashboardID = strFromPtr(resp.DashboardID)
	m.Schedule = types.StringValue(resp.Schedule)
	m.Recipients = setFromCSVPtr(ctx, resp.RecipientEmails, diags.d)
	m.TimeRange = types.StringValue(resp.TimeRange)
	m.SglQuery = strFromPtr(resp.SglQuery)
	m.LastGeneratedAt = strFromPtr(resp.LastGeneratedAt)
	m.NextScheduledAt = strFromPtr(resp.NextScheduledAt)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
}

func (r *scheduledReportResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scheduledReportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toCreateBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	// The create endpoint returns the full report entity.
	var created scheduledReportResponse
	path := r.client.Tenantf("/analytics", "/reports")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating scheduled report", err.Error())
		return
	}

	plan.applyResponse(ctx, &created, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scheduledReportResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scheduledReportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No GET-by-id endpoint exists; list the tenant's reports and find ours.
	var listed []scheduledReportResponse
	listPath := r.client.Tenantf("/analytics", "/reports")
	if err := r.client.Get(ctx, listPath, nil, &listed); err != nil {
		resp.Diagnostics.AddError("Error reading scheduled report", err.Error())
		return
	}

	id := state.ID.ValueString()
	var found *scheduledReportResponse
	for i := range listed {
		if listed[i].ID == id {
			found = &listed[i]
			break
		}
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, found, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *scheduledReportResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan scheduledReportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toUpdateBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	// The patch endpoint returns the full updated report entity.
	var got scheduledReportResponse
	path := r.client.Tenantf("/analytics", "/reports/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, &got); err != nil {
		resp.Diagnostics.AddError("Error updating scheduled report", err.Error())
		return
	}

	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scheduledReportResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scheduledReportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/analytics", "/reports/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting scheduled report", err.Error())
	}
}

func (r *scheduledReportResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
