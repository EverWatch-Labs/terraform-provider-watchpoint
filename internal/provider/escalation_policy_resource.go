package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An escalation policy walks an ordered set of steps when an alert stays
// unacknowledged, paging on-call schedules, individual users, or fanning out to
// another alert profile at each step. POST returns only {id,name}; GET-by-id
// returns the full policy + steps, so Create re-GETs to populate state. PATCH
// replaces the step collection wholesale, mirroring the alerting API.

var (
	_ resource.Resource                = (*escalationPolicyResource)(nil)
	_ resource.ResourceWithConfigure   = (*escalationPolicyResource)(nil)
	_ resource.ResourceWithImportState = (*escalationPolicyResource)(nil)
)

// NewEscalationPolicyResource is the resource constructor wired into the provider.
func NewEscalationPolicyResource() resource.Resource { return &escalationPolicyResource{} }

type escalationPolicyResource struct {
	client *client.Client
}

// escalationPolicyModel maps schema attributes <-> Terraform state.
type escalationPolicyModel struct {
	ID                    types.String          `tfsdk:"id"`
	Name                  types.String          `tfsdk:"name"`
	Description           types.String          `tfsdk:"description"`
	IsEnabled             types.Bool            `tfsdk:"is_enabled"`
	RepeatLastStep        types.Bool            `tfsdk:"repeat_last_step"`
	RepeatIntervalMinutes types.Int64           `tfsdk:"repeat_interval_minutes"`
	MaxRepeats            types.Int64           `tfsdk:"max_repeats"`
	Steps                 []escalationStepModel `tfsdk:"steps"`
	CreatedAt             types.String          `tfsdk:"created_at"`
	UpdatedAt             types.String          `tfsdk:"updated_at"`
}

type escalationStepModel struct {
	StepOrder            types.Int64  `tfsdk:"step_order"`
	DelayMinutes         types.Int64  `tfsdk:"delay_minutes"`
	TargetType           types.String `tfsdk:"target_type"`
	OnCallScheduleID     types.String `tfsdk:"oncall_schedule_id"`
	TargetUserID         types.String `tfsdk:"target_user_id"`
	TargetUserEmail      types.String `tfsdk:"target_user_email"`
	TargetUserName       types.String `tfsdk:"target_user_name"`
	NotifyAlertProfileID types.String `tfsdk:"notify_alert_profile_id"`
}

// --- Wire DTOs (camelCase JSON matching the alerting API) ---

type escalationStepBody struct {
	StepOrder            *int64  `json:"stepOrder,omitempty"`
	DelayMinutes         *int64  `json:"delayMinutes,omitempty"`
	TargetType           string  `json:"targetType"`
	OnCallScheduleID     *string `json:"onCallScheduleId,omitempty"`
	TargetUserID         *string `json:"targetUserId,omitempty"`
	TargetUserEmail      *string `json:"targetUserEmail,omitempty"`
	TargetUserName       *string `json:"targetUserName,omitempty"`
	NotifyAlertProfileID *string `json:"notifyAlertProfileId,omitempty"`
}

type escalationPolicyBody struct {
	Name                  *string              `json:"name,omitempty"`
	Description           *string              `json:"description,omitempty"`
	IsEnabled             *bool                `json:"isEnabled,omitempty"`
	RepeatLastStep        *bool                `json:"repeatLastStep,omitempty"`
	RepeatIntervalMinutes *int64               `json:"repeatIntervalMinutes,omitempty"`
	MaxRepeats            *int64               `json:"maxRepeats,omitempty"`
	Steps                 []escalationStepBody `json:"steps,omitempty"`
}

type escalationStepResponse struct {
	ID                   string  `json:"id"`
	StepOrder            int64   `json:"stepOrder"`
	DelayMinutes         int64   `json:"delayMinutes"`
	TargetType           string  `json:"targetType"`
	OnCallScheduleID     *string `json:"onCallScheduleId"`
	TargetUserID         *string `json:"targetUserId"`
	TargetUserEmail      *string `json:"targetUserEmail"`
	TargetUserName       *string `json:"targetUserName"`
	NotifyAlertProfileID *string `json:"notifyAlertProfileId"`
}

type escalationPolicyResponse struct {
	ID                    string                   `json:"id"`
	Name                  string                   `json:"name"`
	Description           *string                  `json:"description"`
	IsEnabled             bool                     `json:"isEnabled"`
	RepeatLastStep        bool                     `json:"repeatLastStep"`
	RepeatIntervalMinutes int64                    `json:"repeatIntervalMinutes"`
	MaxRepeats            int64                    `json:"maxRepeats"`
	CreatedAt             string                   `json:"createdAt"`
	UpdatedAt             string                   `json:"updatedAt"`
	Steps                 []escalationStepResponse `json:"steps"`
}

func (r *escalationPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_policy"
}

func (r *escalationPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *escalationPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An escalation policy walked when an alert stays unacknowledged. Each ordered " +
			"step pages an on-call schedule, a specific user, or another alert profile after a delay.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Escalation policy ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable policy name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Optional:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the policy is enabled. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"repeat_last_step": schema.BoolAttribute{
				MarkdownDescription: "Keep re-paging the final step until the alert is acknowledged. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"repeat_interval_minutes": schema.Int64Attribute{
				MarkdownDescription: "Minutes between repeats of the final step when `repeat_last_step` is set. Defaults to `15`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(15),
			},
			"max_repeats": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of times the final step is repeated. Defaults to `4`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(4),
			},
			"steps": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered escalation steps. At least one step is required.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"step_order": schema.Int64Attribute{
							MarkdownDescription: "Order the step runs in (ascending). Server-assigned when omitted.",
							Optional:            true,
							Computed:            true,
						},
						"delay_minutes": schema.Int64Attribute{
							MarkdownDescription: "Minutes to wait before this step fires. Defaults to `0` (immediate).",
							Optional:            true,
							Computed:            true,
							Default:             int64default.StaticInt64(0),
						},
						"target_type": schema.StringAttribute{
							MarkdownDescription: "Step target. One of `OnCallSchedule`, `User`, `AlertProfile`.",
							Required:            true,
						},
						"oncall_schedule_id": schema.StringAttribute{
							MarkdownDescription: "On-call schedule to page (required when `target_type` is `OnCallSchedule`).",
							Optional:            true,
						},
						"target_user_id": schema.StringAttribute{
							MarkdownDescription: "User ID to page (used when `target_type` is `User`).",
							Optional:            true,
						},
						"target_user_email": schema.StringAttribute{
							MarkdownDescription: "User email to page (used when `target_type` is `User`).",
							Optional:            true,
						},
						"target_user_name": schema.StringAttribute{
							MarkdownDescription: "Display name for the paged user.",
							Optional:            true,
						},
						"notify_alert_profile_id": schema.StringAttribute{
							MarkdownDescription: "Alert profile to fan out to (required when `target_type` is `AlertProfile`).",
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

func (m *escalationPolicyModel) toBody() *escalationPolicyBody {
	body := &escalationPolicyBody{
		Name:                  strPtr(m.Name),
		Description:           strPtr(m.Description),
		IsEnabled:             boolPtr(m.IsEnabled),
		RepeatLastStep:        boolPtr(m.RepeatLastStep),
		RepeatIntervalMinutes: int64Ptr(m.RepeatIntervalMinutes),
		MaxRepeats:            int64Ptr(m.MaxRepeats),
	}
	if m.Steps != nil {
		body.Steps = make([]escalationStepBody, 0, len(m.Steps))
		for _, s := range m.Steps {
			body.Steps = append(body.Steps, escalationStepBody{
				StepOrder:            int64Ptr(s.StepOrder),
				DelayMinutes:         int64Ptr(s.DelayMinutes),
				TargetType:           s.TargetType.ValueString(),
				OnCallScheduleID:     strPtr(s.OnCallScheduleID),
				TargetUserID:         strPtr(s.TargetUserID),
				TargetUserEmail:      strPtr(s.TargetUserEmail),
				TargetUserName:       strPtr(s.TargetUserName),
				NotifyAlertProfileID: strPtr(s.NotifyAlertProfileID),
			})
		}
	}
	return body
}

func (m *escalationPolicyModel) applyResponse(resp *escalationPolicyResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.RepeatLastStep = types.BoolValue(resp.RepeatLastStep)
	m.RepeatIntervalMinutes = types.Int64Value(resp.RepeatIntervalMinutes)
	m.MaxRepeats = types.Int64Value(resp.MaxRepeats)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)

	if resp.Steps == nil {
		m.Steps = nil
		return
	}
	steps := make([]escalationStepModel, 0, len(resp.Steps))
	for _, s := range resp.Steps {
		steps = append(steps, escalationStepModel{
			StepOrder:            types.Int64Value(s.StepOrder),
			DelayMinutes:         types.Int64Value(s.DelayMinutes),
			TargetType:           types.StringValue(s.TargetType),
			OnCallScheduleID:     strFromPtr(s.OnCallScheduleID),
			TargetUserID:         strFromPtr(s.TargetUserID),
			TargetUserEmail:      strFromPtr(s.TargetUserEmail),
			TargetUserName:       strFromPtr(s.TargetUserName),
			NotifyAlertProfileID: strFromPtr(s.NotifyAlertProfileID),
		})
	}
	m.Steps = steps
}

func (r *escalationPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan escalationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created idNameResponse
	path := r.client.Tenantf("/alerting", "/escalation-policies")
	if err := r.client.Post(ctx, path, plan.toBody(), &created); err != nil {
		resp.Diagnostics.AddError("Error creating escalation policy", err.Error())
		return
	}

	// POST returns only {id,name}; re-read to populate computed fields.
	var got escalationPolicyResponse
	getPath := r.client.Tenantf("/alerting", "/escalation-policies/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading escalation policy after create", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *escalationPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state escalationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got escalationPolicyResponse
	getPath := r.client.Tenantf("/alerting", "/escalation-policies/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading escalation policy", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *escalationPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan escalationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// PATCH returns a status message, so re-GET to refresh state.
	path := r.client.Tenantf("/alerting", "/escalation-policies/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, plan.toBody(), nil); err != nil {
		resp.Diagnostics.AddError("Error updating escalation policy", err.Error())
		return
	}

	var got escalationPolicyResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading escalation policy after update", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *escalationPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state escalationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/alerting", "/escalation-policies/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting escalation policy", err.Error())
	}
}

func (r *escalationPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
