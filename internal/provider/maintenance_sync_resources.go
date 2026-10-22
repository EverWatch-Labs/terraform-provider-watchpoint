package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Maintenance windows synced from outside (Monitoring, /maintenance): ICS
// calendar feeds and ServiceNow / Jira change-ticket webhooks. Both share the
// scope fields and have no GET by id, so Read lists and selects.

var maintenanceScopeTypes = []string{"group", "tag", "service", "monitors"}

func maintenanceScopeAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"scope_type": schema.StringAttribute{
			MarkdownDescription: "What the synced windows cover: `group`, `tag`, `service` or `monitors`.",
			Required:            true,
			Validators:          []validator.String{stringvalidator.OneOf(maintenanceScopeTypes...)},
		},
		"scope_value": schema.StringAttribute{
			MarkdownDescription: "The group ID, tag, service ID, or comma-separated monitor IDs, matching `scope_type`.",
			Required:            true,
			Validators:          []validator.String{stringvalidator.LengthBetween(1, 2048)},
		},
		"notify_on_start": schema.BoolAttribute{
			MarkdownDescription: "Notify when a synced window starts. Defaults to `true`.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
		},
		"notify_on_end": schema.BoolAttribute{
			MarkdownDescription: "Notify when a synced window ends. Defaults to `true`.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
		},
		"enabled": schema.BoolAttribute{
			MarkdownDescription: "Whether syncing is on. Defaults to `true`.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
		},
	}
}

// findByID lists path and returns the row with the given id, or nil.
func findByID[T any](ctx context.Context, c *client.Client, path, id string, idOf func(*T) string) (*T, error) {
	var rows []T
	if err := c.Get(ctx, path, nil, &rows); err != nil {
		return nil, err
	}
	for i := range rows {
		if idOf(&rows[i]) == id {
			return &rows[i], nil
		}
	}
	return nil, nil
}

// ---- watchpoint_maintenance_calendar_feed ----

var (
	_ resource.Resource                = (*calendarFeedResource)(nil)
	_ resource.ResourceWithConfigure   = (*calendarFeedResource)(nil)
	_ resource.ResourceWithImportState = (*calendarFeedResource)(nil)
)

// NewCalendarFeedResource is the resource constructor wired into the provider.
func NewCalendarFeedResource() resource.Resource { return &calendarFeedResource{} }

type calendarFeedResource struct {
	client *client.Client
}

type calendarFeedModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	URL                 types.String `tfsdk:"url"`
	URLHost             types.String `tfsdk:"url_host"`
	ScopeType           types.String `tfsdk:"scope_type"`
	ScopeValue          types.String `tfsdk:"scope_value"`
	SummaryContains     types.String `tfsdk:"summary_contains"`
	PollIntervalMinutes types.Int64  `tfsdk:"poll_interval_minutes"`
	NotifyOnStart       types.Bool   `tfsdk:"notify_on_start"`
	NotifyOnEnd         types.Bool   `tfsdk:"notify_on_end"`
	Enabled             types.Bool   `tfsdk:"enabled"`
}

type calendarFeedBody struct {
	Name                string  `json:"name"`
	URL                 *string `json:"url,omitempty"`
	ScopeType           string  `json:"scopeType"`
	ScopeValue          string  `json:"scopeValue"`
	SummaryContains     *string `json:"summaryContains"`
	PollIntervalMinutes *int64  `json:"pollIntervalMinutes,omitempty"`
	NotifyOnStart       *bool   `json:"notifyOnStart,omitempty"`
	NotifyOnEnd         *bool   `json:"notifyOnEnd,omitempty"`
	Enabled             *bool   `json:"enabled,omitempty"`
}

type calendarFeedResponse struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	URLHost             string  `json:"urlHost"`
	ScopeType           string  `json:"scopeType"`
	ScopeValue          string  `json:"scopeValue"`
	SummaryContains     *string `json:"summaryContains"`
	PollIntervalMinutes int64   `json:"pollIntervalMinutes"`
	NotifyOnStart       bool    `json:"notifyOnStart"`
	NotifyOnEnd         bool    `json:"notifyOnEnd"`
	Enabled             bool    `json:"enabled"`
}

func (r *calendarFeedResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_calendar_feed"
}

func (r *calendarFeedResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *calendarFeedResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := maintenanceScopeAttributes()
	attrs["id"] = schema.StringAttribute{
		MarkdownDescription: "Feed ID.",
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
	attrs["name"] = schema.StringAttribute{
		MarkdownDescription: "Display name (120 characters max).",
		Required:            true,
		Validators:          []validator.String{stringvalidator.LengthBetween(1, 120)},
	}
	attrs["url"] = schema.StringAttribute{
		MarkdownDescription: "ICS feed URL (`https://` or `webcal://`). Sensitive and write-only: the API returns only " +
			"its host (`url_host`), so a URL changed outside Terraform is not detected.",
		Required:  true,
		Sensitive: true,
	}
	attrs["url_host"] = schema.StringAttribute{
		MarkdownDescription: "Host of the stored feed URL.",
		Computed:            true,
	}
	attrs["summary_contains"] = schema.StringAttribute{
		MarkdownDescription: "Only events whose summary contains this text become windows.",
		Optional:            true,
		Validators:          []validator.String{stringvalidator.LengthAtMost(200)},
	}
	attrs["poll_interval_minutes"] = schema.Int64Attribute{
		MarkdownDescription: "Minutes between polls, 5 to 1440. Defaults to `15`.",
		Optional:            true,
		Computed:            true,
		Default:             int64default.StaticInt64(15),
		Validators:          []validator.Int64{int64validator.Between(5, 1440)},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates maintenance windows from an ICS calendar (Outlook, Google Calendar, any iCal " +
			"feed), polled on a schedule. Destroying the feed deletes its future scheduled windows. Requires the " +
			"maintenance calendar sync feature and `monitors.update`.",
		Attributes: attrs,
	}
}

func (m *calendarFeedModel) toBody(withURL bool) *calendarFeedBody {
	b := &calendarFeedBody{
		Name: m.Name.ValueString(), ScopeType: m.ScopeType.ValueString(), ScopeValue: m.ScopeValue.ValueString(),
		SummaryContains: strPtrClearing(m.SummaryContains), PollIntervalMinutes: int64Ptr(m.PollIntervalMinutes),
		NotifyOnStart: boolPtr(m.NotifyOnStart), NotifyOnEnd: boolPtr(m.NotifyOnEnd), Enabled: boolPtr(m.Enabled),
	}
	if withURL {
		b.URL = strPtr(m.URL)
	}
	return b
}

func (m *calendarFeedModel) applyResponse(in *calendarFeedResponse) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.URLHost = types.StringValue(in.URLHost)
	m.ScopeType = types.StringValue(in.ScopeType)
	m.ScopeValue = types.StringValue(in.ScopeValue)
	m.SummaryContains = strFromPtr(in.SummaryContains)
	m.PollIntervalMinutes = types.Int64Value(in.PollIntervalMinutes)
	m.NotifyOnStart = types.BoolValue(in.NotifyOnStart)
	m.NotifyOnEnd = types.BoolValue(in.NotifyOnEnd)
	m.Enabled = types.BoolValue(in.Enabled)
}

func (r *calendarFeedResource) path(suffix string) string {
	return r.client.Tenantf("/monitoring", "/maintenance/calendar-feeds%s", suffix)
}

func (r *calendarFeedResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan calendarFeedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got calendarFeedResponse
	if err := r.client.Post(ctx, r.path(""), plan.toBody(true), &got); err != nil {
		resp.Diagnostics.AddError("Error creating calendar feed", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *calendarFeedResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state calendarFeedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := findByID(ctx, r.client, r.path(""), state.ID.ValueString(), func(f *calendarFeedResponse) string { return f.ID })
	if err != nil {
		resp.Diagnostics.AddError("Error reading calendar feed", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update sends the URL only when it changed: a resent URL resets the feed's conditional-GET cache.
func (r *calendarFeedResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior calendarFeedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got calendarFeedResponse
	if err := r.client.Patch(ctx, r.path("/"+plan.ID.ValueString()), plan.toBody(!plan.URL.Equal(prior.URL)), &got); err != nil {
		resp.Diagnostics.AddError("Error updating calendar feed", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *calendarFeedResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state calendarFeedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path("/"+state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting calendar feed", err.Error())
	}
}

func (r *calendarFeedResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}

// ---- watchpoint_maintenance_change_source ----

var (
	_ resource.Resource                = (*changeSourceResource)(nil)
	_ resource.ResourceWithConfigure   = (*changeSourceResource)(nil)
	_ resource.ResourceWithImportState = (*changeSourceResource)(nil)
)

// NewChangeSourceResource is the resource constructor wired into the provider.
func NewChangeSourceResource() resource.Resource { return &changeSourceResource{} }

type changeSourceResource struct {
	client *client.Client
}

type changeSourceModel struct {
	ID              types.String `tfsdk:"id"`
	Kind            types.String `tfsdk:"kind"`
	Name            types.String `tfsdk:"name"`
	ScopeType       types.String `tfsdk:"scope_type"`
	ScopeValue      types.String `tfsdk:"scope_value"`
	ApprovedStates  types.Set    `tfsdk:"approved_states"`
	CancelledStates types.Set    `tfsdk:"cancelled_states"`
	StartField      types.String `tfsdk:"start_field"`
	EndField        types.String `tfsdk:"end_field"`
	NotifyOnStart   types.Bool   `tfsdk:"notify_on_start"`
	NotifyOnEnd     types.Bool   `tfsdk:"notify_on_end"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	WebhookURL      types.String `tfsdk:"webhook_url"`
	Secret          types.String `tfsdk:"secret"`
	SecretHint      types.String `tfsdk:"secret_hint"`
}

type changeSourceBody struct {
	Kind            *string  `json:"kind,omitempty"`
	Name            string   `json:"name"`
	ScopeType       string   `json:"scopeType"`
	ScopeValue      string   `json:"scopeValue"`
	ApprovedStates  []string `json:"approvedStates,omitempty"`
	CancelledStates []string `json:"cancelledStates,omitempty"`
	StartField      *string  `json:"startField,omitempty"`
	EndField        *string  `json:"endField,omitempty"`
	NotifyOnStart   *bool    `json:"notifyOnStart,omitempty"`
	NotifyOnEnd     *bool    `json:"notifyOnEnd,omitempty"`
	Enabled         *bool    `json:"enabled,omitempty"`
}

type changeSourceResponse struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Name            string   `json:"name"`
	SecretHint      string   `json:"secretHint"`
	WebhookURL      string   `json:"webhookUrl"`
	ScopeType       string   `json:"scopeType"`
	ScopeValue      string   `json:"scopeValue"`
	ApprovedStates  []string `json:"approvedStates"`
	CancelledStates []string `json:"cancelledStates"`
	StartField      *string  `json:"startField"`
	EndField        *string  `json:"endField"`
	NotifyOnStart   bool     `json:"notifyOnStart"`
	NotifyOnEnd     bool     `json:"notifyOnEnd"`
	Enabled         bool     `json:"enabled"`
	Secret          *string  `json:"secret"`
}

func (r *changeSourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_change_source"
}

func (r *changeSourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *changeSourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	stateSet := func(desc string) schema.SetAttribute {
		return schema.SetAttribute{
			MarkdownDescription: desc, Optional: true, Computed: true, ElementType: types.StringType,
			PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			Validators:    []validator.Set{setvalidator.SizeBetween(1, 20)},
		}
	}
	field := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			MarkdownDescription: desc, Optional: true, Computed: true,
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			Validators:    []validator.String{stringvalidator.LengthBetween(1, 128)},
		}
	}
	attrs := maintenanceScopeAttributes()
	attrs["id"] = schema.StringAttribute{
		MarkdownDescription: "Source ID.",
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
	attrs["kind"] = schema.StringAttribute{
		MarkdownDescription: "`servicenow` or `jira`. Changing it replaces the source (and its secret).",
		Required:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		Validators:          []validator.String{stringvalidator.OneOf("servicenow", "jira")},
	}
	attrs["name"] = schema.StringAttribute{
		MarkdownDescription: "Display name (120 characters max).",
		Required:            true,
		Validators:          []validator.String{stringvalidator.LengthBetween(1, 120)},
	}
	attrs["approved_states"] = stateSet("Ticket states that create or update a window. Defaults to `Scheduled`, " +
		"`Implement` (ServiceNow) or `Approved`, `Scheduled`, `Implementing` (Jira).")
	attrs["cancelled_states"] = stateSet("Ticket states that cancel the window. Defaults to `Canceled`, `Cancelled` " +
		"(ServiceNow) or `Cancelled`, `Declined`, `Rejected` (Jira).")
	attrs["start_field"] = field("Ticket field holding the start time. ServiceNow defaults to `start_date`; required for " +
		"Jira (a custom field id such as `customfield_10020`).")
	attrs["end_field"] = field("Ticket field holding the end time. ServiceNow defaults to `end_date`; required for Jira.")
	attrs["webhook_url"] = schema.StringAttribute{
		MarkdownDescription: "URL to configure in ServiceNow or Jira as the webhook target.",
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
	attrs["secret"] = schema.StringAttribute{
		MarkdownDescription: "HMAC signing secret the ticket system signs each webhook with. Returned only when the " +
			"source is created, so it is kept in state (sensitive); an imported source has no secret here.",
		Computed:      true,
		Sensitive:     true,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
	attrs["secret_hint"] = schema.StringAttribute{
		MarkdownDescription: "Last characters of the signing secret.",
		Computed:            true,
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates maintenance windows from approved ServiceNow or Jira change tickets delivered by " +
			"signed webhook to `webhook_url`. Requires the maintenance calendar sync feature; creating and " +
			"destroying need `settings.manage`.",
		Attributes: attrs,
	}
}

func (m *changeSourceModel) toBody(ctx context.Context, ds *diagSink) *changeSourceBody {
	return &changeSourceBody{
		Name: m.Name.ValueString(), ScopeType: m.ScopeType.ValueString(), ScopeValue: m.ScopeValue.ValueString(),
		ApprovedStates:  stringSliceFromSet(ctx, m.ApprovedStates, ds.d),
		CancelledStates: stringSliceFromSet(ctx, m.CancelledStates, ds.d),
		StartField:      strPtr(m.StartField), EndField: strPtr(m.EndField),
		NotifyOnStart: boolPtr(m.NotifyOnStart), NotifyOnEnd: boolPtr(m.NotifyOnEnd), Enabled: boolPtr(m.Enabled),
	}
}

func (m *changeSourceModel) applyResponse(ctx context.Context, in *changeSourceResponse, ds *diagSink) {
	m.ID = types.StringValue(in.ID)
	m.Kind = types.StringValue(in.Kind)
	m.Name = types.StringValue(in.Name)
	m.ScopeType = types.StringValue(in.ScopeType)
	m.ScopeValue = types.StringValue(in.ScopeValue)
	m.ApprovedStates = setFromStringSlice(ctx, in.ApprovedStates, ds.d)
	m.CancelledStates = setFromStringSlice(ctx, in.CancelledStates, ds.d)
	m.StartField = strFromPtr(in.StartField)
	m.EndField = strFromPtr(in.EndField)
	m.NotifyOnStart = types.BoolValue(in.NotifyOnStart)
	m.NotifyOnEnd = types.BoolValue(in.NotifyOnEnd)
	m.Enabled = types.BoolValue(in.Enabled)
	m.WebhookURL = types.StringValue(in.WebhookURL)
	m.SecretHint = types.StringValue(in.SecretHint)
	if in.Secret != nil {
		m.Secret = types.StringValue(*in.Secret)
	} else if m.Secret.IsUnknown() {
		m.Secret = types.StringNull()
	}
}

func (r *changeSourceResource) path(suffix string) string {
	return r.client.Tenantf("/monitoring", "/maintenance/change-sources%s", suffix)
}

func (r *changeSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan changeSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}
	body.Kind = strPtr(plan.Kind)
	var got changeSourceResponse
	if err := r.client.Post(ctx, r.path(""), body, &got); err != nil {
		resp.Diagnostics.AddError("Error creating change source", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *changeSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state changeSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := findByID(ctx, r.client, r.path(""), state.ID.ValueString(), func(s *changeSourceResponse) string { return s.ID })
	if err != nil {
		resp.Diagnostics.AddError("Error reading change source", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(ctx, got, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *changeSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan changeSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}
	var got changeSourceResponse
	if err := r.client.Patch(ctx, r.path("/"+plan.ID.ValueString()), body, &got); err != nil {
		resp.Diagnostics.AddError("Error updating change source", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *changeSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state changeSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path("/"+state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting change source", err.Error())
	}
}

func (r *changeSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
