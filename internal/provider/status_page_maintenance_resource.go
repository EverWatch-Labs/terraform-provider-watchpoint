package provider

import (
	"context"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// status_page_maintenance is planned work announced on a status page, optionally
// repeating. It is owned by the Status API (Watchpoint.Status.Api
// MaintenanceWindowsController) and is NOT the same entity as
// watchpoint_maintenance_window, which is the Monitoring API's monitor-pausing
// window. This one announces work to visitors and can suppress alerts for the
// monitors behind the components it names; that one pauses monitors directly.
//
// ALL TIMES ARE UTC. The API takes and returns bare UTC instants with no
// timezone field, so a series scheduled at 02:00 stays at 02:00 UTC across a
// daylight-saving change.
//
// RECURRENCE is RRULE-lite and materialised, not expanded at render time: the
// server sweep writes ONE NEW ROW per occurrence, one occurrence ahead. This
// resource owns the first occurrence, the series anchor, and the rule on it.
// Later occurrences are rows Terraform does not manage, so destroying this
// resource removes the anchor and stops the series, but leaves occurrences the
// sweep has already written on the page.
//
// The whole recurrence rule is sent on every write, including
// recurrence_frequency = "None", because the API only re-evaluates the rule when
// a frequency is present. Omitting it would make removing recurrence from
// configuration a silent no-op.
//
// recurrence_by_day and component_ids are unordered membership, so both are Sets
// and never Lists: the API returns days in weekday order and components in
// insertion order, neither of which matches configuration order.
//
// status is READ ONLY. A background sweep drives Scheduled -> InProgress ->
// Completed, so a configurable status would make every plan after a window
// starts propose dragging it back to Scheduled. Cancelling a window is an
// imperative action, done in the portal or through the API.

var (
	_ resource.Resource                = (*statusPageMaintenanceResource)(nil)
	_ resource.ResourceWithConfigure   = (*statusPageMaintenanceResource)(nil)
	_ resource.ResourceWithImportState = (*statusPageMaintenanceResource)(nil)
)

// maintenanceRecurrenceFrequencies are the MaintenanceRecurrenceFrequency names
// the API accepts. "None" is the one-off default.
var maintenanceRecurrenceFrequencies = []string{"None", "Daily", "Weekly", "Monthly"}

// maintenanceByDayCodes are the two-letter weekday codes the API normalises to.
// They are upper-case on the wire, so the validator pins the spelling rather than
// letting a lower-case config round-trip changed and fail the apply.
var maintenanceByDayCodes = []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

// NewStatusPageMaintenanceResource is the resource constructor wired into the provider.
func NewStatusPageMaintenanceResource() resource.Resource { return &statusPageMaintenanceResource{} }

type statusPageMaintenanceResource struct {
	client *client.Client
}

// statusPageMaintenanceModel maps schema attributes <-> Terraform state.
type statusPageMaintenanceModel struct {
	ID                  types.String `tfsdk:"id"`
	StatusPageID        types.String `tfsdk:"status_page_id"`
	Title               types.String `tfsdk:"title"`
	DescriptionMarkdown types.String `tfsdk:"description_markdown"`
	ScheduledStartAt    types.String `tfsdk:"scheduled_start_at"`
	ScheduledEndAt      types.String `tfsdk:"scheduled_end_at"`
	SuppressAlerts      types.Bool   `tfsdk:"suppress_alerts"`
	ComponentIDs        types.Set    `tfsdk:"component_ids"`
	RecurrenceFrequency types.String `tfsdk:"recurrence_frequency"`
	RecurrenceInterval  types.Int64  `tfsdk:"recurrence_interval"`
	RecurrenceByDay     types.Set    `tfsdk:"recurrence_by_day"`
	RecurrenceUntil     types.String `tfsdk:"recurrence_until"`
	RecurrenceCount     types.Int64  `tfsdk:"recurrence_count"`
	Status              types.String `tfsdk:"status"`
	ActualStartAt       types.String `tfsdk:"actual_start_at"`
	ActualEndAt         types.String `tfsdk:"actual_end_at"`
	CreatedAt           types.String `tfsdk:"created_at"`
	UpdatedAt           types.String `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the status API) ---

// statusPageMaintenanceBody serves both CreateRequest and PatchRequest. Create
// requires title + both instants, which the schema enforces; the recurrence
// fields and componentIds are always sent so each write replaces the rule and
// the component list wholesale.
type statusPageMaintenanceBody struct {
	Title               *string  `json:"title,omitempty"`
	DescriptionMarkdown *string  `json:"descriptionMarkdown,omitempty"`
	ScheduledStartAt    *string  `json:"scheduledStartAt,omitempty"`
	ScheduledEndAt      *string  `json:"scheduledEndAt,omitempty"`
	SuppressAlerts      *bool    `json:"suppressAlerts,omitempty"`
	ComponentIDs        []string `json:"componentIds"`
	RecurrenceFrequency *string  `json:"recurrenceFrequency,omitempty"`
	RecurrenceInterval  *int64   `json:"recurrenceInterval,omitempty"`
	RecurrenceByDay     *string  `json:"recurrenceByDay,omitempty"`
	RecurrenceUntil     *string  `json:"recurrenceUntil,omitempty"`
	RecurrenceCount     *int64   `json:"recurrenceCount,omitempty"`
}

// statusPageMaintenanceRecurrence mirrors the nested `recurrence` object, which
// the API returns as null for a one-off window.
type statusPageMaintenanceRecurrence struct {
	Frequency string  `json:"frequency"`
	Interval  int64   `json:"interval"`
	ByDay     *string `json:"byDay"`
	Until     *string `json:"until"`
	Count     *int64  `json:"count"`
}

type statusPageMaintenanceResponse struct {
	ID                  string                           `json:"id"`
	StatusPageID        string                           `json:"statusPageId"`
	Title               string                           `json:"title"`
	DescriptionMarkdown *string                          `json:"descriptionMarkdown"`
	ScheduledStartAt    string                           `json:"scheduledStartAt"`
	ScheduledEndAt      string                           `json:"scheduledEndAt"`
	ActualStartAt       *string                          `json:"actualStartAt"`
	ActualEndAt         *string                          `json:"actualEndAt"`
	Status              string                           `json:"status"`
	SuppressAlerts      bool                             `json:"suppressAlerts"`
	ComponentIDs        []string                         `json:"componentIds"`
	Recurrence          *statusPageMaintenanceRecurrence `json:"recurrence"`
	CreatedAt           string                           `json:"createdAt"`
	UpdatedAt           string                           `json:"updatedAt"`
}

func (r *statusPageMaintenanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_maintenance"
}

func (r *statusPageMaintenanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *statusPageMaintenanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Planned work announced on a status page, optionally recurring. Every instant is " +
			"UTC. This is the status-page announcement, not the monitor-pausing window: use " +
			"`watchpoint_maintenance_window` to pause monitors. A recurring window is materialised one " +
			"occurrence ahead by the server, and this resource owns the first occurrence and the rule on it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Maintenance window ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status_page_id": schema.StringAttribute{
				MarkdownDescription: "ID of the status page this window is announced on. Changing it replaces the window.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"title": schema.StringAttribute{
				MarkdownDescription: "Window title shown to visitors, for example `Database upgrade`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 300)},
			},
			"description_markdown": schema.StringAttribute{
				MarkdownDescription: "Markdown body rendered under the maintenance banner on the public page.",
				Optional:            true,
			},
			"scheduled_start_at": schema.StringAttribute{
				MarkdownDescription: "Window start (RFC 3339, UTC). For a recurring window this is the first occurrence.",
				Required:            true,
			},
			"scheduled_end_at": schema.StringAttribute{
				MarkdownDescription: "Window end (RFC 3339, UTC). Must be after `scheduled_start_at`, and for a " +
					"recurring window must be shorter than the gap between occurrences.",
				Required: true,
			},
			"suppress_alerts": schema.BoolAttribute{
				MarkdownDescription: "Skip alert notifications for the monitors behind `component_ids` while the " +
					"window is running. Defaults to `true`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"component_ids": schema.SetAttribute{
				MarkdownDescription: "Components this work affects. Unordered. Drives both what the public page " +
					"marks as under maintenance and, with `suppress_alerts`, which monitors stay quiet.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"recurrence_frequency": schema.StringAttribute{
				MarkdownDescription: "How often the window repeats: `" +
					strings.Join(maintenanceRecurrenceFrequencies, "`, `") + "`. Defaults to `None`, a one-off window.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString("None"),
				Validators: []validator.String{stringvalidator.OneOf(maintenanceRecurrenceFrequencies...)},
			},
			"recurrence_interval": schema.Int64Attribute{
				MarkdownDescription: "Repeat every N periods, so `2` with `Weekly` is fortnightly. Between 1 and 52. Defaults to `1`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
				Validators:          []validator.Int64{int64validator.Between(1, 52)},
			},
			"recurrence_by_day": schema.SetAttribute{
				MarkdownDescription: "Weekly recurrence only: the weekdays the window falls on, as `" +
					strings.Join(maintenanceByDayCodes, "`, `") + "`. Unordered. Omit to repeat on the same " +
					"weekday as the first occurrence. The API clears this for any frequency other than `Weekly`, " +
					"so setting it alongside `Daily` or `Monthly` will fail the apply.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf(maintenanceByDayCodes...)),
				},
			},
			"recurrence_until": schema.StringAttribute{
				MarkdownDescription: "Stop repeating once the next occurrence would start after this instant " +
					"(RFC 3339, UTC). Must be after `scheduled_start_at`. Omit for no end date.",
				Optional: true,
			},
			"recurrence_count": schema.Int64Attribute{
				MarkdownDescription: "Total number of occurrences in the series, including the first. Omit for unbounded.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current window status (`Scheduled`, `InProgress`, `Completed`, `Cancelled`). " +
					"Driven by the server, not configurable: cancelling a window is done in the portal.",
				Computed: true,
			},
			"actual_start_at": schema.StringAttribute{
				MarkdownDescription: "When the window actually started (RFC 3339), once it has.",
				Computed:            true,
			},
			"actual_end_at": schema.StringAttribute{
				MarkdownDescription: "When the window actually ended (RFC 3339), once it has.",
				Computed:            true,
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

func (m *statusPageMaintenanceModel) toBody(ctx context.Context, diags *diagSink) *statusPageMaintenanceBody {
	return &statusPageMaintenanceBody{
		Title:               strPtr(m.Title),
		DescriptionMarkdown: strPtr(m.DescriptionMarkdown),
		ScheduledStartAt:    strPtr(m.ScheduledStartAt),
		ScheduledEndAt:      strPtr(m.ScheduledEndAt),
		SuppressAlerts:      boolPtr(m.SuppressAlerts),
		// Never nil: an omitted list means "leave the components alone", which
		// would make removing them from configuration a silent no-op.
		ComponentIDs:        emptyIfNil(stringSliceFromSet(ctx, m.ComponentIDs, diags.d)),
		RecurrenceFrequency: strPtr(m.RecurrenceFrequency),
		RecurrenceInterval:  int64Ptr(m.RecurrenceInterval),
		RecurrenceByDay:     csvPtrFromSet(ctx, m.RecurrenceByDay, diags.d),
		RecurrenceUntil:     strPtr(m.RecurrenceUntil),
		RecurrenceCount:     int64Ptr(m.RecurrenceCount),
	}
}

// applyResponse folds a window into state. A one-off window comes back with a
// null `recurrence`, which has to be mapped onto the schema's defaults ("None"
// and interval 1) rather than left null: a Computed attribute that resolves to
// null when the plan said "None" fails the apply.
func (m *statusPageMaintenanceModel) applyResponse(ctx context.Context, resp *statusPageMaintenanceResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.StatusPageID = types.StringValue(resp.StatusPageID)
	m.Title = types.StringValue(resp.Title)
	m.DescriptionMarkdown = strFromPtr(resp.DescriptionMarkdown)
	m.ScheduledStartAt = types.StringValue(resp.ScheduledStartAt)
	m.ScheduledEndAt = types.StringValue(resp.ScheduledEndAt)
	m.SuppressAlerts = types.BoolValue(resp.SuppressAlerts)
	m.ComponentIDs = setOrNull(ctx, resp.ComponentIDs, diags)
	if resp.Recurrence == nil {
		m.RecurrenceFrequency = types.StringValue("None")
		m.RecurrenceInterval = types.Int64Value(1)
		m.RecurrenceByDay = types.SetNull(types.StringType)
		m.RecurrenceUntil = types.StringNull()
		m.RecurrenceCount = types.Int64Null()
	} else {
		m.RecurrenceFrequency = types.StringValue(resp.Recurrence.Frequency)
		m.RecurrenceInterval = types.Int64Value(resp.Recurrence.Interval)
		m.RecurrenceByDay = setFromCSVPtr(ctx, resp.Recurrence.ByDay, diags.d)
		m.RecurrenceUntil = strFromPtr(resp.Recurrence.Until)
		m.RecurrenceCount = int64FromPtr(resp.Recurrence.Count)
	}
	m.Status = types.StringValue(resp.Status)
	m.ActualStartAt = strFromPtr(resp.ActualStartAt)
	m.ActualEndAt = strFromPtr(resp.ActualEndAt)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
}

func (r *statusPageMaintenanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageMaintenanceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var created statusPageMaintenanceResponse
	path := r.client.Tenantf("/status", "/pages/%s/maintenance", plan.StatusPageID.ValueString())
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating status page maintenance window", err.Error())
		return
	}

	plan.applyResponse(ctx, &created, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageMaintenanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageMaintenanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got statusPageMaintenanceResponse
	path := r.client.Tenantf("/status", "/pages/%s/maintenance/%s",
		state.StatusPageID.ValueString(), state.ID.ValueString())
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading status page maintenance window", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageMaintenanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageMaintenanceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated statusPageMaintenanceResponse
	path := r.client.Tenantf("/status", "/pages/%s/maintenance/%s",
		plan.StatusPageID.ValueString(), plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, &updated); err != nil {
		resp.Diagnostics.AddError("Error updating status page maintenance window", err.Error())
		return
	}

	plan.applyResponse(ctx, &updated, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageMaintenanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageMaintenanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/status", "/pages/%s/maintenance/%s",
		state.StatusPageID.ValueString(), state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting status page maintenance window", err.Error())
	}
}

// ImportState takes "<status_page_id>/<maintenance_window_id>": a window is
// addressed through its page, so the page id has to come with it.
func (r *statusPageMaintenanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the form \"<status_page_id>/<maintenance_window_id>\", got: "+req.ID,
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
