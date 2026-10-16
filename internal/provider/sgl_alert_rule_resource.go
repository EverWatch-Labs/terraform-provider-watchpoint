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

// SGL alert rules evaluate a scheduled SGL query and raise an alert when the
// result crosses a threshold. POST returns only {id,name}; GET-by-id and PATCH
// return the full rule entity, so Create re-GETs to populate computed fields.

var (
	_ resource.Resource                = (*sglAlertRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*sglAlertRuleResource)(nil)
	_ resource.ResourceWithImportState = (*sglAlertRuleResource)(nil)
)

// NewSGLAlertRuleResource is the resource constructor wired into the provider.
func NewSGLAlertRuleResource() resource.Resource { return &sglAlertRuleResource{} }

type sglAlertRuleResource struct {
	client *client.Client
}

// sglAlertRuleModel maps schema attributes <-> Terraform state.
type sglAlertRuleModel struct {
	ID               types.String  `tfsdk:"id"`
	Name             types.String  `tfsdk:"name"`
	Description      types.String  `tfsdk:"description"`
	IsEnabled        types.Bool    `tfsdk:"is_enabled"`
	SglQuery         types.String  `tfsdk:"sgl_query"`
	Operator         types.String  `tfsdk:"operator"`
	Threshold        types.Float64 `tfsdk:"threshold"`
	IntervalSeconds  types.Int64   `tfsdk:"interval_seconds"`
	Severity         types.String  `tfsdk:"severity"`
	TimeRange        types.String  `tfsdk:"time_range"`
	AlertProfileID   types.String  `tfsdk:"alert_profile_id"`
	OwnerGroupID     types.String  `tfsdk:"owner_group_id"`
	LastEvaluatedAt  types.String  `tfsdk:"last_evaluated_at"`
	LastResultValue  types.Float64 `tfsdk:"last_result_value"`
	LastResultStatus types.String  `tfsdk:"last_result_status"`
	CreatedAt        types.String  `tfsdk:"created_at"`
	UpdatedAt        types.String  `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the alerting API) ---

type sglAlertRuleCreateBody struct {
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	SglQuery        string  `json:"sglQuery"`
	Operator        *string `json:"operator,omitempty"`
	Threshold       float64 `json:"threshold"`
	IntervalSeconds int64   `json:"intervalSeconds"`
	Severity        string  `json:"severity"`
	TimeRange       *string `json:"timeRange,omitempty"`
	AlertProfileID  *string `json:"alertProfileId,omitempty"`
	IsEnabled       *bool   `json:"isEnabled,omitempty"`
	OwnerGroupID    *string `json:"ownerGroupId,omitempty"`
}

// sglAlertRuleUpdateBody mirrors the PATCH DTO; pointers omit unchanged fields.
type sglAlertRuleUpdateBody struct {
	Name            *string  `json:"name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	SglQuery        *string  `json:"sglQuery,omitempty"`
	Operator        *string  `json:"operator,omitempty"`
	Threshold       *float64 `json:"threshold,omitempty"`
	IntervalSeconds *int64   `json:"intervalSeconds,omitempty"`
	Severity        *string  `json:"severity,omitempty"`
	TimeRange       *string  `json:"timeRange,omitempty"`
	AlertProfileID  *string  `json:"alertProfileId,omitempty"`
	IsEnabled       *bool    `json:"isEnabled,omitempty"`
	// "" clears the owner; omitted keeps it.
	OwnerGroupID *string `json:"ownerGroupId,omitempty"`
}

type sglAlertRuleResponse struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      *string  `json:"description"`
	IsEnabled        bool     `json:"isEnabled"`
	SglQuery         string   `json:"sglQuery"`
	Operator         string   `json:"operator"`
	Threshold        float64  `json:"threshold"`
	IntervalSeconds  int64    `json:"intervalSeconds"`
	Severity         string   `json:"severity"`
	TimeRange        string   `json:"timeRange"`
	AlertProfileID   *string  `json:"alertProfileId"`
	OwnerGroupID     *string  `json:"ownerGroupId"`
	LastEvaluatedAt  *string  `json:"lastEvaluatedAt"`
	LastResultValue  *float64 `json:"lastResultValue"`
	LastResultStatus *string  `json:"lastResultStatus"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
}

func (r *sglAlertRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sgl_alert_rule"
}

func (r *sglAlertRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *sglAlertRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An SGL alert rule that periodically evaluates an SGL query and alerts when the " +
			"result crosses a threshold.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "SGL alert rule ID.",
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
			"sgl_query": schema.StringAttribute{
				MarkdownDescription: "The SGL query to evaluate. Required.",
				Required:            true,
			},
			"operator": schema.StringAttribute{
				MarkdownDescription: "Comparison operator applied to the query result against `threshold` " +
					"(e.g. `gt`, `lt`, `gte`, `lte`, `eq`). Defaults to `gt`.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("gt"),
			},
			"threshold": schema.Float64Attribute{
				MarkdownDescription: "Threshold the query result is compared against.",
				Required:            true,
			},
			"interval_seconds": schema.Int64Attribute{
				MarkdownDescription: "Evaluation interval in seconds (clamped server-side to 60-86400). Defaults to `300`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(300),
			},
			"severity": schema.StringAttribute{
				MarkdownDescription: "Severity raised on a breach. One of `Success`, `Warning`, `Critical`, " +
					"`Error`, `Timeout`.",
				Required: true,
			},
			"time_range": schema.StringAttribute{
				MarkdownDescription: "Lookback window for the query (e.g. `1h`, `24h`). Defaults to `1h`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("1h"),
			},
			"alert_profile_id": schema.StringAttribute{
				MarkdownDescription: "Alert profile used to dispatch notifications. Omit for status-only tracking.",
				Optional:            true,
			},
			"owner_group_id": schema.StringAttribute{
				MarkdownDescription: "User group (`watchpoint_user_group`) that owns this rule. A filter label " +
					"only; it grants no access.",
				Optional: true,
			},
			"last_evaluated_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp the rule was last evaluated (RFC 3339).",
				Computed:            true,
			},
			"last_result_value": schema.Float64Attribute{
				MarkdownDescription: "Most recent query result value.",
				Computed:            true,
			},
			"last_result_status": schema.StringAttribute{
				MarkdownDescription: "Most recent evaluation status.",
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

func (m *sglAlertRuleModel) toCreateBody() *sglAlertRuleCreateBody {
	return &sglAlertRuleCreateBody{
		Name:            m.Name.ValueString(),
		Description:     strPtr(m.Description),
		SglQuery:        m.SglQuery.ValueString(),
		Operator:        strPtr(m.Operator),
		Threshold:       m.Threshold.ValueFloat64(),
		IntervalSeconds: m.IntervalSeconds.ValueInt64(),
		Severity:        m.Severity.ValueString(),
		TimeRange:       strPtr(m.TimeRange),
		AlertProfileID:  strPtr(m.AlertProfileID),
		IsEnabled:       boolPtr(m.IsEnabled),
		OwnerGroupID:    strPtr(m.OwnerGroupID),
	}
}

func (m *sglAlertRuleModel) toUpdateBody() *sglAlertRuleUpdateBody {
	return &sglAlertRuleUpdateBody{
		Name:            strPtr(m.Name),
		Description:     strPtr(m.Description),
		SglQuery:        strPtr(m.SglQuery),
		Operator:        strPtr(m.Operator),
		Threshold:       float64Ptr(m.Threshold),
		IntervalSeconds: int64Ptr(m.IntervalSeconds),
		Severity:        strPtr(m.Severity),
		TimeRange:       strPtr(m.TimeRange),
		AlertProfileID:  strPtr(m.AlertProfileID),
		IsEnabled:       boolPtr(m.IsEnabled),
		OwnerGroupID:    strPtrClearing(m.OwnerGroupID),
	}
}

// applyResponse copies API response fields into the model.
func (m *sglAlertRuleModel) applyResponse(resp *sglAlertRuleResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.SglQuery = types.StringValue(resp.SglQuery)
	m.Operator = types.StringValue(resp.Operator)
	m.Threshold = types.Float64Value(resp.Threshold)
	m.IntervalSeconds = types.Int64Value(resp.IntervalSeconds)
	m.Severity = types.StringValue(resp.Severity)
	m.TimeRange = types.StringValue(resp.TimeRange)
	m.AlertProfileID = strFromPtr(resp.AlertProfileID)
	m.OwnerGroupID = strFromPtr(resp.OwnerGroupID)
	m.LastEvaluatedAt = strFromPtr(resp.LastEvaluatedAt)
	m.LastResultValue = float64FromPtr(resp.LastResultValue)
	m.LastResultStatus = strFromPtr(resp.LastResultStatus)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
}

func (r *sglAlertRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sglAlertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toCreateBody()

	var created idNameResponse
	path := r.client.Tenantf("/alerting", "/sgl-alert-rules")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating SGL alert rule", err.Error())
		return
	}

	// POST returns only {id,name}; re-read for computed/clamped fields.
	var got sglAlertRuleResponse
	getPath := r.client.Tenantf("/alerting", "/sgl-alert-rules/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading SGL alert rule after create", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sglAlertRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sglAlertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got sglAlertRuleResponse
	getPath := r.client.Tenantf("/alerting", "/sgl-alert-rules/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading SGL alert rule", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *sglAlertRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sglAlertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toUpdateBody()

	// PATCH returns the full rule entity.
	var got sglAlertRuleResponse
	path := r.client.Tenantf("/alerting", "/sgl-alert-rules/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, &got); err != nil {
		resp.Diagnostics.AddError("Error updating SGL alert rule", err.Error())
		return
	}

	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sglAlertRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sglAlertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/alerting", "/sgl-alert-rules/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting SGL alert rule", err.Error())
	}
}

func (r *sglAlertRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
