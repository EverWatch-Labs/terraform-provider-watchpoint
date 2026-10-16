package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An alert routing rule answers "alerts that look like THIS go to THOSE
// people". Conditions inside a rule are ANDed, separate rules OR, and every
// matching rule contributes its targets unless it sets stop_processing.
// Evaluation order is the explicit priority (ascending), never the order the
// rules were written in, so a rule can be inserted between two existing ones
// without recreating anything.
//
// conditions and targets are SETS, not lists: they are unordered semantically
// (AND / union) and the API returns them in a content-derived order rather
// than insertion order, which a list would report as "inconsistent result
// after apply" on the very first read. Same rule as region_ids and tags.
//
// POST and PATCH both return the full rule, so neither needs a follow-up GET.

var (
	_ resource.Resource                = (*alertRoutingRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*alertRoutingRuleResource)(nil)
	_ resource.ResourceWithImportState = (*alertRoutingRuleResource)(nil)
)

// Enum names accepted by the alerting API, validated here so a typo fails at
// plan time instead of arriving as an opaque JSON deserialization 400.
var (
	alertRoutingConditionTypes = []string{"MonitorTag", "MonitorGroup", "MonitorType", "Severity", "ServiceName", "TimeWindow"}
	alertRoutingOperators      = []string{"In", "NotIn", "Contains", "NotContains"}
	alertRoutingTargetTypes    = []string{"AlertProfile", "Channel", "EscalationPolicy"}
)

// NewAlertRoutingRuleResource is the resource constructor wired into the provider.
func NewAlertRoutingRuleResource() resource.Resource { return &alertRoutingRuleResource{} }

type alertRoutingRuleResource struct {
	client *client.Client
}

// alertRoutingRuleModel maps schema attributes <-> Terraform state.
type alertRoutingRuleModel struct {
	ID             types.String                 `tfsdk:"id"`
	Name           types.String                 `tfsdk:"name"`
	Description    types.String                 `tfsdk:"description"`
	IsEnabled      types.Bool                   `tfsdk:"is_enabled"`
	Priority       types.Int64                  `tfsdk:"priority"`
	StopProcessing types.Bool                   `tfsdk:"stop_processing"`
	OwnerGroupID   types.String                 `tfsdk:"owner_group_id"`
	Conditions     []alertRoutingConditionModel `tfsdk:"conditions"`
	Targets        []alertRoutingTargetModel    `tfsdk:"targets"`
	CreatedAt      types.String                 `tfsdk:"created_at"`
	UpdatedAt      types.String                 `tfsdk:"updated_at"`
}

type alertRoutingConditionModel struct {
	ConditionType    types.String `tfsdk:"condition_type"`
	Operator         types.String `tfsdk:"operator"`
	Values           types.Set    `tfsdk:"values"`
	TimeZone         types.String `tfsdk:"time_zone"`
	DaysOfWeek       types.Set    `tfsdk:"days_of_week"`
	StartMinuteOfDay types.Int64  `tfsdk:"start_minute_of_day"`
	EndMinuteOfDay   types.Int64  `tfsdk:"end_minute_of_day"`
}

type alertRoutingTargetModel struct {
	TargetType types.String `tfsdk:"target_type"`
	TargetID   types.String `tfsdk:"target_id"`
}

// --- Wire DTOs (camelCase JSON matching the alerting API) ---

type alertRoutingConditionBody struct {
	ConditionType    string  `json:"conditionType"`
	Operator         string  `json:"operator"`
	Value            *string `json:"value,omitempty"`
	TimeZone         *string `json:"timeZone,omitempty"`
	DaysOfWeek       *string `json:"daysOfWeek,omitempty"`
	StartMinuteOfDay *int64  `json:"startMinuteOfDay,omitempty"`
	EndMinuteOfDay   *int64  `json:"endMinuteOfDay,omitempty"`
}

type alertRoutingTargetBody struct {
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
}

// alertRoutingRuleBody serves both POST and PATCH. Conditions and targets carry
// no `omitempty`: the API replaces them wholesale when the key is present and
// leaves them alone when it is absent, so a nil slice (marshalled as null) means
// "leave unchanged" and an empty slice means "clear them".
type alertRoutingRuleBody struct {
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	IsEnabled      *bool   `json:"isEnabled,omitempty"`
	Priority       *int64  `json:"priority,omitempty"`
	StopProcessing *bool   `json:"stopProcessing,omitempty"`
	// Always sent: "" is the API's clear signal on PATCH and "no owner" on POST.
	OwnerGroupID *string                     `json:"ownerGroupId"`
	Conditions   []alertRoutingConditionBody `json:"conditions"`
	Targets      []alertRoutingTargetBody    `json:"targets"`
}

type alertRoutingConditionResponse struct {
	ID               string  `json:"id"`
	ConditionType    string  `json:"conditionType"`
	Operator         string  `json:"operator"`
	Value            *string `json:"value"`
	TimeZone         *string `json:"timeZone"`
	DaysOfWeek       *string `json:"daysOfWeek"`
	StartMinuteOfDay *int64  `json:"startMinuteOfDay"`
	EndMinuteOfDay   *int64  `json:"endMinuteOfDay"`
}

type alertRoutingTargetResponse struct {
	ID         string `json:"id"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
}

type alertRoutingRuleResponse struct {
	ID             string                          `json:"id"`
	Name           string                          `json:"name"`
	Description    *string                         `json:"description"`
	IsEnabled      bool                            `json:"isEnabled"`
	Priority       int64                           `json:"priority"`
	StopProcessing bool                            `json:"stopProcessing"`
	OwnerGroupID   *string                         `json:"ownerGroupId"`
	CreatedAt      string                          `json:"createdAt"`
	UpdatedAt      string                          `json:"updatedAt"`
	Conditions     []alertRoutingConditionResponse `json:"conditions"`
	Targets        []alertRoutingTargetResponse    `json:"targets"`
}

func (r *alertRoutingRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_routing_rule"
}

func (r *alertRoutingRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *alertRoutingRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A rule that routes matching alerts to extra audiences. Conditions are ANDed " +
			"within a rule and ORed across rules; every matching rule contributes its targets unless it sets " +
			"`stop_processing`. An alert that matches nothing (or whose targets resolve to no enabled channel) " +
			"still goes to the tenant's default alert profile, so a rule can only ever add reach, never remove " +
			"it. Requires the `alert_routing_rules` feature (SMB plan and above).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Alert routing rule ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable rule name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Optional:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the rule is evaluated. Defaults to `true`. A disabled rule never matches.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"priority": schema.Int64Attribute{
				MarkdownDescription: "Evaluation order, ascending: `100` is evaluated before `200`. Defaults to " +
					"the end of the list (highest existing priority plus 10). Ties break on creation time, so " +
					"the order is stable across replicas.",
				Optional: true,
				Computed: true,
			},
			"stop_processing": schema.BoolAttribute{
				MarkdownDescription: "Stop evaluating lower-priority rules once this one matches. Defaults to " +
					"`false`, so several rules can each contribute targets to the same alert.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"owner_group_id": schema.StringAttribute{
				MarkdownDescription: "User group (`watchpoint_user_group`) that owns this rule. A filter label " +
					"only; it grants no access. The API rejects a group that does not exist in the tenant.",
				Optional: true,
			},
			"conditions": schema.SetNestedAttribute{
				MarkdownDescription: "Conditions that must ALL match (AND) for the rule to fire. A rule with no " +
					"conditions is a catch-all. Unordered.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"condition_type": schema.StringAttribute{
							MarkdownDescription: "What to match on. One of `MonitorTag`, `MonitorGroup`, " +
								"`MonitorType`, `Severity`, `ServiceName`, `TimeWindow`.",
							Required:   true,
							Validators: []validator.String{stringvalidator.OneOf(alertRoutingConditionTypes...)},
						},
						"operator": schema.StringAttribute{
							MarkdownDescription: "How `values` is compared against the alert. `In` / `NotIn` are " +
								"case-insensitive exact membership, `Contains` / `NotContains` are substring " +
								"tests. Every operator is evaluated against the whole candidate set (a monitor " +
								"has many tags), so `NotIn` means none of them is listed. For a `TimeWindow`, " +
								"`In` means inside the window and `NotIn` outside it. Defaults to `In`.",
							Optional:   true,
							Computed:   true,
							Default:    stringdefault.StaticString("In"),
							Validators: []validator.String{stringvalidator.OneOf(alertRoutingOperators...)},
						},
						"values": schema.SetAttribute{
							MarkdownDescription: "Candidate values to compare against, unordered. Required for " +
								"every `condition_type` except `TimeWindow`, which ignores it. Sent to the API " +
								"as a comma-separated list, so a value containing a comma cannot be expressed.",
							Optional:    true,
							ElementType: types.StringType,
						},
						"time_zone": schema.StringAttribute{
							MarkdownDescription: "IANA zone id the window is evaluated in, for example " +
								"`America/New_York`. Required for `TimeWindow` (an unknown id is rejected), " +
								"ignored otherwise.",
							Optional: true,
						},
						"days_of_week": schema.SetAttribute{
							MarkdownDescription: "Days the window applies to, `0` = Sunday through `6` = " +
								"Saturday. Omit for every day. `TimeWindow` only. Unordered.",
							Optional:    true,
							ElementType: types.Int64Type,
						},
						"start_minute_of_day": schema.Int64Attribute{
							MarkdownDescription: "Local minute of day the window opens, `0`-`1439` (`540` is " +
								"09:00). Omit for `0`. `TimeWindow` only.",
							Optional:   true,
							Validators: []validator.Int64{int64validator.Between(0, 1439)},
						},
						"end_minute_of_day": schema.Int64Attribute{
							MarkdownDescription: "Local minute of day the window closes, `0`-`1439`, exclusive. " +
								"An end below the start wraps past midnight and belongs to the day it opened " +
								"on, so a Friday 22:00-06:00 window still matches at 02:00 on Saturday. Equal " +
								"start and end mean the full 24 hours, which is how a day-of-week-only rule is " +
								"written. `TimeWindow` only.",
							Optional:   true,
							Validators: []validator.Int64{int64validator.Between(0, 1439)},
						},
					},
				},
			},
			"targets": schema.SetNestedAttribute{
				MarkdownDescription: "Who additionally hears about a matching alert. The channels of every " +
					"matching rule are unioned and de-duplicated. Unordered.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"target_type": schema.StringAttribute{
							MarkdownDescription: "One of `AlertProfile` (all of that profile's enabled " +
								"channels), `Channel` (one channel on any of the tenant's profiles), or " +
								"`EscalationPolicy` (overrides the policy the alert would have escalated " +
								"through; only the first one in priority order is used).",
							Required:   true,
							Validators: []validator.String{stringvalidator.OneOf(alertRoutingTargetTypes...)},
						},
						"target_id": schema.StringAttribute{
							MarkdownDescription: "Id of the alert profile, channel or escalation policy. This " +
								"is deliberately not a foreign key server-side: a target pointing at something " +
								"that was deleted contributes nothing rather than taking the rule down with it.",
							Required: true,
						},
					},
				},
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

func (m *alertRoutingRuleModel) toBody(ctx context.Context, diags *diagSink) *alertRoutingRuleBody {
	body := &alertRoutingRuleBody{
		Name:           strPtr(m.Name),
		Description:    strPtr(m.Description),
		IsEnabled:      boolPtr(m.IsEnabled),
		Priority:       int64Ptr(m.Priority),
		StopProcessing: boolPtr(m.StopProcessing),
		OwnerGroupID:   strPtrClearing(m.OwnerGroupID),
	}
	if m.Conditions != nil {
		body.Conditions = make([]alertRoutingConditionBody, 0, len(m.Conditions))
		for _, c := range m.Conditions {
			body.Conditions = append(body.Conditions, alertRoutingConditionBody{
				ConditionType:    c.ConditionType.ValueString(),
				Operator:         c.Operator.ValueString(),
				Value:            csvPtrFromSet(ctx, c.Values, diags.d),
				TimeZone:         strPtr(c.TimeZone),
				DaysOfWeek:       csvPtrFromInt64Set(ctx, c.DaysOfWeek, diags.d),
				StartMinuteOfDay: int64Ptr(c.StartMinuteOfDay),
				EndMinuteOfDay:   int64Ptr(c.EndMinuteOfDay),
			})
		}
	}
	if m.Targets != nil {
		body.Targets = make([]alertRoutingTargetBody, 0, len(m.Targets))
		for _, t := range m.Targets {
			body.Targets = append(body.Targets, alertRoutingTargetBody{
				TargetType: t.TargetType.ValueString(),
				TargetID:   t.TargetID.ValueString(),
			})
		}
	}
	return body
}

func (m *alertRoutingRuleModel) applyResponse(ctx context.Context, resp *alertRoutingRuleResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.Priority = types.Int64Value(resp.Priority)
	m.StopProcessing = types.BoolValue(resp.StopProcessing)
	m.OwnerGroupID = strFromPtr(resp.OwnerGroupID)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)

	// An empty collection is left exactly as the plan (or prior state) holds it.
	// The API returns `[]` for both "no conditions were sent" and "an empty set
	// was sent", but Terraform treats a null set and an empty set as different
	// values and fails the apply if the answer flips. Whichever the caller wrote
	// is the right one to keep.
	if len(resp.Conditions) > 0 {
		conditions := make([]alertRoutingConditionModel, 0, len(resp.Conditions))
		for _, c := range resp.Conditions {
			conditions = append(conditions, alertRoutingConditionModel{
				ConditionType:    types.StringValue(c.ConditionType),
				Operator:         types.StringValue(c.Operator),
				Values:           setFromCSVPtr(ctx, c.Value, diags.d),
				TimeZone:         strFromPtr(c.TimeZone),
				DaysOfWeek:       int64SetFromCSVPtr(ctx, c.DaysOfWeek, diags.d),
				StartMinuteOfDay: int64FromPtr(c.StartMinuteOfDay),
				EndMinuteOfDay:   int64FromPtr(c.EndMinuteOfDay),
			})
		}
		m.Conditions = conditions
	}

	if len(resp.Targets) > 0 {
		targets := make([]alertRoutingTargetModel, 0, len(resp.Targets))
		for _, t := range resp.Targets {
			targets = append(targets, alertRoutingTargetModel{
				TargetType: types.StringValue(t.TargetType),
				TargetID:   types.StringValue(t.TargetID),
			})
		}
		m.Targets = targets
	}
}

func (r *alertRoutingRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertRoutingRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var created alertRoutingRuleResponse
	path := r.client.Tenantf("/alerting", "/alert-routing-rules")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating alert routing rule", err.Error())
		return
	}

	plan.applyResponse(ctx, &created, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *alertRoutingRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertRoutingRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got alertRoutingRuleResponse
	path := r.client.Tenantf("/alerting", "/alert-routing-rules/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading alert routing rule", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *alertRoutingRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertRoutingRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var got alertRoutingRuleResponse
	path := r.client.Tenantf("/alerting", "/alert-routing-rules/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, &got); err != nil {
		resp.Diagnostics.AddError("Error updating alert routing rule", err.Error())
		return
	}

	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *alertRoutingRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertRoutingRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/alerting", "/alert-routing-rules/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting alert routing rule", err.Error())
	}
}

func (r *alertRoutingRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
