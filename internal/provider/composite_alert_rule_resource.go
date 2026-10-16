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

// Composite alert rules fire when a combination of monitor conditions trips
// together (AND / OR / N-of-M logic). The alerting API returns the full mapped
// rule (including its conditions) on POST/PATCH/GET.
//
// DEPRECATED. The platform stopped evaluating composite rules and removed the
// portal surface; the API is retained only so existing state keeps applying.
// The resource and its CRUD stay in place because deleting them is the breaking
// change this deprecation exists to avoid: the warning has to ship in a release
// before the endpoint can be retired.

// compositeAlertRuleDeprecation is the framework deprecation warning raised on
// plan/apply. Shared by the resource and the data source so both say the same
// thing, and repeated in each MarkdownDescription because tfplugindocs renders
// attribute-level deprecation only, never a schema-level DeprecationMessage.
const compositeAlertRuleDeprecation = "watchpoint_composite_alert_rule is deprecated and will be removed in a " +
	"future release. Composite rules are no longer evaluated by the platform, so an existing rule never fires. " +
	"Express the same combination as an SGL alert rule and use watchpoint_sgl_alert_rule instead " +
	"(Alerting, /alerting/rules)."

var (
	_ resource.Resource                = (*compositeAlertRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*compositeAlertRuleResource)(nil)
	_ resource.ResourceWithImportState = (*compositeAlertRuleResource)(nil)
)

// NewCompositeAlertRuleResource is the resource constructor wired into the provider.
func NewCompositeAlertRuleResource() resource.Resource { return &compositeAlertRuleResource{} }

type compositeAlertRuleResource struct {
	client *client.Client
}

// compositeAlertRuleModel maps schema attributes <-> Terraform state.
type compositeAlertRuleModel struct {
	ID                types.String                       `tfsdk:"id"`
	Name              types.String                       `tfsdk:"name"`
	Description       types.String                       `tfsdk:"description"`
	IsEnabled         types.Bool                         `tfsdk:"is_enabled"`
	Logic             types.String                       `tfsdk:"logic"`
	MinFailCount      types.Int64                        `tfsdk:"min_fail_count"`
	TimeWindowSeconds types.Int64                        `tfsdk:"time_window_seconds"`
	AlertProfileID    types.String                       `tfsdk:"alert_profile_id"`
	AlertProfileName  types.String                       `tfsdk:"alert_profile_name"`
	CooldownMinutes   types.Int64                        `tfsdk:"cooldown_minutes"`
	Status            types.String                       `tfsdk:"status"`
	LastTriggeredAt   types.String                       `tfsdk:"last_triggered_at"`
	LastEvaluatedAt   types.String                       `tfsdk:"last_evaluated_at"`
	Conditions        []compositeAlertRuleConditionModel `tfsdk:"conditions"`
	CreatedAt         types.String                       `tfsdk:"created_at"`
	UpdatedAt         types.String                       `tfsdk:"updated_at"`
}

type compositeAlertRuleConditionModel struct {
	MonitorID                 types.String `tfsdk:"monitor_id"`
	MonitorType               types.String `tfsdk:"monitor_type"`
	MonitorName               types.String `tfsdk:"monitor_name"`
	FailStatus                types.String `tfsdk:"fail_status"`
	TimeWindowOverrideSeconds types.Int64  `tfsdk:"time_window_override_seconds"`
}

// --- Wire DTOs (camelCase JSON matching the alerting API) ---

type compositeAlertRuleConditionBody struct {
	MonitorID                 string  `json:"monitorId"`
	MonitorType               string  `json:"monitorType"`
	MonitorName               *string `json:"monitorName,omitempty"`
	FailStatus                string  `json:"failStatus"`
	TimeWindowOverrideSeconds *int64  `json:"timeWindowOverrideSeconds,omitempty"`
}

type compositeAlertRuleCreateBody struct {
	Name              string                            `json:"name"`
	Description       *string                           `json:"description,omitempty"`
	IsEnabled         *bool                             `json:"isEnabled,omitempty"`
	Logic             *string                           `json:"logic,omitempty"`
	MinFailCount      *int64                            `json:"minFailCount,omitempty"`
	TimeWindowSeconds *int64                            `json:"timeWindowSeconds,omitempty"`
	AlertProfileID    *string                           `json:"alertProfileId,omitempty"`
	CooldownMinutes   *int64                            `json:"cooldownMinutes,omitempty"`
	Conditions        []compositeAlertRuleConditionBody `json:"conditions"`
}

// compositeAlertRuleUpdateBody mirrors the PATCH DTO.
type compositeAlertRuleUpdateBody = compositeAlertRuleCreateBody

type compositeAlertRuleConditionResponse struct {
	ID                        string  `json:"id"`
	MonitorID                 string  `json:"monitorId"`
	MonitorType               string  `json:"monitorType"`
	MonitorName               *string `json:"monitorName"`
	FailStatus                string  `json:"failStatus"`
	TimeWindowOverrideSeconds *int64  `json:"timeWindowOverrideSeconds"`
}

type compositeAlertRuleResponse struct {
	ID                string                                `json:"id"`
	Name              string                                `json:"name"`
	Description       *string                               `json:"description"`
	IsEnabled         bool                                  `json:"isEnabled"`
	Logic             string                                `json:"logic"`
	MinFailCount      int64                                 `json:"minFailCount"`
	TimeWindowSeconds int64                                 `json:"timeWindowSeconds"`
	AlertProfileID    *string                               `json:"alertProfileId"`
	AlertProfileName  *string                               `json:"alertProfileName"`
	Status            string                                `json:"status"`
	LastTriggeredAt   *string                               `json:"lastTriggeredAt"`
	LastEvaluatedAt   *string                               `json:"lastEvaluatedAt"`
	CooldownMinutes   int64                                 `json:"cooldownMinutes"`
	CreatedAt         string                                `json:"createdAt"`
	UpdatedAt         string                                `json:"updatedAt"`
	Conditions        []compositeAlertRuleConditionResponse `json:"conditions"`
}

func (r *compositeAlertRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_composite_alert_rule"
}

func (r *compositeAlertRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *compositeAlertRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		DeprecationMessage: compositeAlertRuleDeprecation,
		MarkdownDescription: "**Deprecated.** A composite alert rule that fires when a combination of monitor " +
			"conditions trips together under AND / OR / N-of-M logic. Composite rules are no longer evaluated by " +
			"the platform, so an existing rule never fires, and this resource will be removed in a future release. " +
			"Express the same combination as an SGL alert rule and use `watchpoint_sgl_alert_rule` instead " +
			"(Alerting, `/alerting/rules`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Composite alert rule ID.",
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
				MarkdownDescription: "Whether the rule is enabled. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"logic": schema.StringAttribute{
				MarkdownDescription: "Combination logic. One of `And`, `Or`, `NOfM`. Defaults to `And`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("And"),
			},
			"min_fail_count": schema.Int64Attribute{
				MarkdownDescription: "Minimum failing conditions required to trip the rule (used with `NOfM`). Defaults to `1`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
			},
			"time_window_seconds": schema.Int64Attribute{
				MarkdownDescription: "Window (seconds) within which conditions must be failing together. Defaults to `0`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
			},
			"alert_profile_id": schema.StringAttribute{
				MarkdownDescription: "Alert profile used to dispatch notifications. Omit for status-only tracking.",
				Optional:            true,
			},
			"alert_profile_name": schema.StringAttribute{
				MarkdownDescription: "Resolved name of the attached alert profile.",
				Computed:            true,
			},
			"cooldown_minutes": schema.Int64Attribute{
				MarkdownDescription: "Cooldown (minutes) between re-triggers. Defaults to `5`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(5),
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Current rule status (`OK`, `Triggered`, `Acknowledged`).",
				Computed:            true,
			},
			"last_triggered_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp the rule last triggered (RFC 3339).",
				Computed:            true,
			},
			"last_evaluated_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp the rule was last evaluated (RFC 3339).",
				Computed:            true,
			},
			"conditions": schema.ListNestedAttribute{
				MarkdownDescription: "Monitor conditions combined by this rule.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"monitor_id": schema.StringAttribute{
							MarkdownDescription: "Monitor whose status feeds this condition.",
							Required:            true,
						},
						"monitor_type": schema.StringAttribute{
							MarkdownDescription: "Monitor type. One of `HttpApi`, `Ssl`, `Synthetic`, `Dns`, `Tcp`, `Ping`. Defaults to `HttpApi`.",
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString("HttpApi"),
						},
						"monitor_name": schema.StringAttribute{
							MarkdownDescription: "Optional cached monitor name for display.",
							Optional:            true,
							Computed:            true,
						},
						"fail_status": schema.StringAttribute{
							MarkdownDescription: "Check status that counts as failing for this condition. One of " +
								"`Success`, `Warning`, `Critical`, `Error`, `Timeout`. Defaults to `Critical`.",
							Optional: true,
							Computed: true,
							Default:  stringdefault.StaticString("Critical"),
						},
						"time_window_override_seconds": schema.Int64Attribute{
							MarkdownDescription: "Optional per-condition override of the rule time window (seconds).",
							Optional:            true,
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

func (m *compositeAlertRuleModel) toBody() *compositeAlertRuleCreateBody {
	conditions := make([]compositeAlertRuleConditionBody, 0, len(m.Conditions))
	for _, c := range m.Conditions {
		conditions = append(conditions, compositeAlertRuleConditionBody{
			MonitorID:                 c.MonitorID.ValueString(),
			MonitorType:               c.MonitorType.ValueString(),
			MonitorName:               strPtr(c.MonitorName),
			FailStatus:                c.FailStatus.ValueString(),
			TimeWindowOverrideSeconds: int64Ptr(c.TimeWindowOverrideSeconds),
		})
	}
	return &compositeAlertRuleCreateBody{
		Name:              m.Name.ValueString(),
		Description:       strPtr(m.Description),
		IsEnabled:         boolPtr(m.IsEnabled),
		Logic:             strPtr(m.Logic),
		MinFailCount:      int64Ptr(m.MinFailCount),
		TimeWindowSeconds: int64Ptr(m.TimeWindowSeconds),
		AlertProfileID:    strPtr(m.AlertProfileID),
		CooldownMinutes:   int64Ptr(m.CooldownMinutes),
		Conditions:        conditions,
	}
}

// applyResponse copies API response fields into the model.
func (m *compositeAlertRuleModel) applyResponse(resp *compositeAlertRuleResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.Logic = types.StringValue(resp.Logic)
	m.MinFailCount = types.Int64Value(resp.MinFailCount)
	m.TimeWindowSeconds = types.Int64Value(resp.TimeWindowSeconds)
	m.AlertProfileID = strFromPtr(resp.AlertProfileID)
	m.AlertProfileName = strFromPtr(resp.AlertProfileName)
	m.CooldownMinutes = types.Int64Value(resp.CooldownMinutes)
	m.Status = types.StringValue(resp.Status)
	m.LastTriggeredAt = strFromPtr(resp.LastTriggeredAt)
	m.LastEvaluatedAt = strFromPtr(resp.LastEvaluatedAt)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)

	conditions := make([]compositeAlertRuleConditionModel, 0, len(resp.Conditions))
	for _, c := range resp.Conditions {
		conditions = append(conditions, compositeAlertRuleConditionModel{
			MonitorID:                 types.StringValue(c.MonitorID),
			MonitorType:               types.StringValue(c.MonitorType),
			MonitorName:               strFromPtr(c.MonitorName),
			FailStatus:                types.StringValue(c.FailStatus),
			TimeWindowOverrideSeconds: int64FromPtr(c.TimeWindowOverrideSeconds),
		})
	}
	m.Conditions = conditions
}

func (r *compositeAlertRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan compositeAlertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toBody()

	// POST returns the full mapped rule (including conditions).
	var created compositeAlertRuleResponse
	path := r.client.Tenantf("/alerting", "/alert-rules")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating composite alert rule", err.Error())
		return
	}

	plan.applyResponse(&created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *compositeAlertRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state compositeAlertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got compositeAlertRuleResponse
	getPath := r.client.Tenantf("/alerting", "/alert-rules/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading composite alert rule", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *compositeAlertRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan compositeAlertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toBody()

	// PATCH returns the full mapped rule.
	var got compositeAlertRuleResponse
	path := r.client.Tenantf("/alerting", "/alert-rules/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, &got); err != nil {
		resp.Diagnostics.AddError("Error updating composite alert rule", err.Error())
		return
	}

	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *compositeAlertRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state compositeAlertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/alerting", "/alert-rules/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting composite alert rule", err.Error())
	}
}

func (r *compositeAlertRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
