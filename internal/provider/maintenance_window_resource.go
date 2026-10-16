package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// maintenanceWindowResource manages a scheduled maintenance window that pauses
// monitors (or groups) for one-time or recurring downtime.
var (
	_ resource.Resource                = (*maintenanceWindowResource)(nil)
	_ resource.ResourceWithConfigure   = (*maintenanceWindowResource)(nil)
	_ resource.ResourceWithImportState = (*maintenanceWindowResource)(nil)
)

// NewMaintenanceWindowResource is the resource constructor wired into the provider.
func NewMaintenanceWindowResource() resource.Resource { return &maintenanceWindowResource{} }

type maintenanceWindowResource struct {
	client *client.Client
}

// maintenanceWindowModel maps schema attributes <-> Terraform state.
type maintenanceWindowModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	StartTimeUTC      types.String `tfsdk:"start_time_utc"`
	EndTimeUTC        types.String `tfsdk:"end_time_utc"`
	DurationMinutes   types.Int64  `tfsdk:"duration_minutes"`
	Timezone          types.String `tfsdk:"timezone"`
	IsRecurring       types.Bool   `tfsdk:"is_recurring"`
	RecurrenceType    types.String `tfsdk:"recurrence_type"`
	RecurrenceDays    types.String `tfsdk:"recurrence_days"`
	RecurrenceHour    types.Int64  `tfsdk:"recurrence_hour"`
	RecurrenceMinute  types.Int64  `tfsdk:"recurrence_minute"`
	RecurrenceEndDate types.String `tfsdk:"recurrence_end_date"`
	MonitorIDs        types.Set    `tfsdk:"monitor_ids"`
	GroupIDs          types.Set    `tfsdk:"group_ids"`
	NotifyOnStart     types.Bool   `tfsdk:"notify_on_start"`
	NotifyOnEnd       types.Bool   `tfsdk:"notify_on_end"`
	Status            types.String `tfsdk:"status"`
	CreatedAt         types.String `tfsdk:"created_at"`
	UpdatedAt         types.String `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

// maintenanceWindowBody serves both the create and patch payloads. The PATCH
// DTO treats null/omitted fields as "leave unchanged"; the create DTO requires
// name + schedule, which the schema enforces.
type maintenanceWindowBody struct {
	Name              *string `json:"name,omitempty"`
	Description       *string `json:"description,omitempty"`
	StartTimeUTC      *string `json:"startTimeUtc,omitempty"`
	EndTimeUTC        *string `json:"endTimeUtc,omitempty"`
	DurationMinutes   *int64  `json:"durationMinutes,omitempty"`
	Timezone          *string `json:"timezone,omitempty"`
	IsRecurring       *bool   `json:"isRecurring,omitempty"`
	RecurrenceType    *string `json:"recurrenceType,omitempty"`
	RecurrenceDays    *string `json:"recurrenceDays,omitempty"`
	RecurrenceHour    *int64  `json:"recurrenceHour,omitempty"`
	RecurrenceMinute  *int64  `json:"recurrenceMinute,omitempty"`
	RecurrenceEndDate *string `json:"recurrenceEndDate,omitempty"`
	MonitorIDs        *string `json:"monitorIds,omitempty"`
	GroupIDs          *string `json:"groupIds,omitempty"`
	NotifyOnStart     *bool   `json:"notifyOnStart,omitempty"`
	NotifyOnEnd       *bool   `json:"notifyOnEnd,omitempty"`
}

type maintenanceWindowResponse struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	StartTimeUTC      string  `json:"startTimeUtc"`
	EndTimeUTC        string  `json:"endTimeUtc"`
	DurationMinutes   int64   `json:"durationMinutes"`
	Timezone          string  `json:"timezone"`
	IsRecurring       bool    `json:"isRecurring"`
	RecurrenceType    *string `json:"recurrenceType"`
	RecurrenceDays    *string `json:"recurrenceDays"`
	RecurrenceHour    int64   `json:"recurrenceHour"`
	RecurrenceMinute  int64   `json:"recurrenceMinute"`
	RecurrenceEndDate *string `json:"recurrenceEndDate"`
	MonitorIDs        *string `json:"monitorIds"`
	GroupIDs          *string `json:"groupIds"`
	NotifyOnStart     bool    `json:"notifyOnStart"`
	NotifyOnEnd       bool    `json:"notifyOnEnd"`
	Status            string  `json:"status"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
}

func (r *maintenanceWindowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

func (r *maintenanceWindowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *maintenanceWindowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A maintenance window pauses the targeted monitors and groups for scheduled downtime, " +
			"either one-time or recurring. The status enum is `Scheduled`, `Active`, `Completed`, `EndedEarly`, `Disabled`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Maintenance window ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable window name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Free-form description.",
				Optional:            true,
			},
			"start_time_utc": schema.StringAttribute{
				MarkdownDescription: "Window start (RFC 3339, UTC). For recurring windows this is the first-occurrence anchor.",
				Required:            true,
			},
			"end_time_utc": schema.StringAttribute{
				MarkdownDescription: "Window end (RFC 3339, UTC). For recurring windows this derives from start + duration.",
				Required:            true,
			},
			"duration_minutes": schema.Int64Attribute{
				MarkdownDescription: "Duration in minutes. Used by recurring windows to compute each occurrence's end.",
				Required:            true,
			},
			"timezone": schema.StringAttribute{
				MarkdownDescription: "IANA timezone ID (e.g. `America/New_York`) in which recurrence is evaluated. Defaults to `UTC`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("UTC"),
			},
			"is_recurring": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a recurring window. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"recurrence_type": schema.StringAttribute{
				MarkdownDescription: "Recurrence type: `daily`, `weekly`, or `monthly`.",
				Optional:            true,
			},
			"recurrence_days": schema.StringAttribute{
				MarkdownDescription: "For weekly: comma-separated days (0=Sun..6=Sat). For monthly: day-of-month (1-31). For daily: empty.",
				Optional:            true,
			},
			"recurrence_hour": schema.Int64Attribute{
				MarkdownDescription: "Hour of day (0-23, in the configured timezone) the recurring window starts. Defaults to `0`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
			},
			"recurrence_minute": schema.Int64Attribute{
				MarkdownDescription: "Minute of the hour (0-59) the recurring window starts. Defaults to `0`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
			},
			"recurrence_end_date": schema.StringAttribute{
				MarkdownDescription: "Date (RFC 3339) after which recurrence stops. Null = forever.",
				Optional:            true,
			},
			"monitor_ids": schema.SetAttribute{
				MarkdownDescription: "Monitor IDs this window applies to. Unordered.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"group_ids": schema.SetAttribute{
				MarkdownDescription: "Monitor group IDs this window applies to. Unordered.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"notify_on_start": schema.BoolAttribute{
				MarkdownDescription: "Send a notification when the window starts. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"notify_on_end": schema.BoolAttribute{
				MarkdownDescription: "Send a notification when the window ends. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current window status (`Scheduled`, `Active`, `Completed`, `EndedEarly`, `Disabled`).",
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

func (m *maintenanceWindowModel) toBody(ctx context.Context, diags *diagSink) *maintenanceWindowBody {
	return &maintenanceWindowBody{
		Name:              strPtr(m.Name),
		Description:       strPtr(m.Description),
		StartTimeUTC:      strPtr(m.StartTimeUTC),
		EndTimeUTC:        strPtr(m.EndTimeUTC),
		DurationMinutes:   int64Ptr(m.DurationMinutes),
		Timezone:          strPtr(m.Timezone),
		IsRecurring:       boolPtr(m.IsRecurring),
		RecurrenceType:    strPtr(m.RecurrenceType),
		RecurrenceDays:    strPtr(m.RecurrenceDays),
		RecurrenceHour:    int64Ptr(m.RecurrenceHour),
		RecurrenceMinute:  int64Ptr(m.RecurrenceMinute),
		RecurrenceEndDate: strPtr(m.RecurrenceEndDate),
		MonitorIDs:        csvPtrFromSet(ctx, m.MonitorIDs, diags.d),
		GroupIDs:          csvPtrFromSet(ctx, m.GroupIDs, diags.d),
		NotifyOnStart:     boolPtr(m.NotifyOnStart),
		NotifyOnEnd:       boolPtr(m.NotifyOnEnd),
	}
}

func (m *maintenanceWindowModel) applyResponse(ctx context.Context, resp *maintenanceWindowResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.StartTimeUTC = types.StringValue(resp.StartTimeUTC)
	m.EndTimeUTC = types.StringValue(resp.EndTimeUTC)
	m.DurationMinutes = types.Int64Value(resp.DurationMinutes)
	m.Timezone = types.StringValue(resp.Timezone)
	m.IsRecurring = types.BoolValue(resp.IsRecurring)
	m.RecurrenceType = strFromPtr(resp.RecurrenceType)
	m.RecurrenceDays = strFromPtr(resp.RecurrenceDays)
	m.RecurrenceHour = types.Int64Value(resp.RecurrenceHour)
	m.RecurrenceMinute = types.Int64Value(resp.RecurrenceMinute)
	m.RecurrenceEndDate = strFromPtr(resp.RecurrenceEndDate)
	m.MonitorIDs = setFromCSVPtr(ctx, resp.MonitorIDs, diags.d)
	m.GroupIDs = setFromCSVPtr(ctx, resp.GroupIDs, diags.d)
	m.NotifyOnStart = types.BoolValue(resp.NotifyOnStart)
	m.NotifyOnEnd = types.BoolValue(resp.NotifyOnEnd)
	m.Status = types.StringValue(resp.Status)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
}

func (r *maintenanceWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var created idNameResponse
	path := r.client.Tenantf("/monitoring", "/maintenance-windows")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating maintenance window", err.Error())
		return
	}

	var got maintenanceWindowResponse
	getPath := r.client.Tenantf("/monitoring", "/maintenance-windows/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading maintenance window after create", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *maintenanceWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got maintenanceWindowResponse
	getPath := r.client.Tenantf("/monitoring", "/maintenance-windows/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading maintenance window", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *maintenanceWindowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/maintenance-windows/%s", plan.ID.ValueString())
	var got maintenanceWindowResponse
	if err := r.client.Patch(ctx, path, body, &got); err != nil {
		resp.Diagnostics.AddError("Error updating maintenance window", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *maintenanceWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/maintenance-windows/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting maintenance window", err.Error())
	}
}

func (r *maintenanceWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
