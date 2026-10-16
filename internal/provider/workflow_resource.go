package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A workflow (Alerting, /workflows): when an event matching `trigger` (and the
// optional condition) fires, run an ordered list of actions. PATCH replaces the
// action list wholesale and returns the full workflow, so Update needs no re-GET.

var (
	_ resource.Resource                = (*workflowResource)(nil)
	_ resource.ResourceWithConfigure   = (*workflowResource)(nil)
	_ resource.ResourceWithImportState = (*workflowResource)(nil)
)

// NewWorkflowResource is the resource constructor wired into the provider.
func NewWorkflowResource() resource.Resource { return &workflowResource{} }

type workflowResource struct {
	client *client.Client
}

type workflowModel struct {
	ID              types.String          `tfsdk:"id"`
	Name            types.String          `tfsdk:"name"`
	Description     types.String          `tfsdk:"description"`
	IsEnabled       types.Bool            `tfsdk:"is_enabled"`
	Trigger         types.String          `tfsdk:"trigger"`
	ConditionJSON   types.String          `tfsdk:"condition_json"`
	CooldownMinutes types.Int64           `tfsdk:"cooldown_minutes"`
	Actions         []workflowActionModel `tfsdk:"actions"`
	CreatedAt       types.String          `tfsdk:"created_at"`
	UpdatedAt       types.String          `tfsdk:"updated_at"`
}

type workflowActionModel struct {
	ActionType types.String `tfsdk:"action_type"`
	ConfigJSON types.String `tfsdk:"config_json"`
}

type workflowActionBody struct {
	OrderIndex int    `json:"orderIndex"`
	ActionType string `json:"actionType"`
	ConfigJSON string `json:"configJson"`
}

// workflowBody serves create and PATCH. Description and the condition are sent
// as "" when removed from config: the API reads null as "leave unchanged".
type workflowBody struct {
	Name            string               `json:"name"`
	Description     *string              `json:"description"`
	IsEnabled       *bool                `json:"isEnabled,omitempty"`
	Trigger         string               `json:"trigger"`
	ConditionJSON   *string              `json:"conditionJson"`
	CooldownMinutes *int64               `json:"cooldownMinutes,omitempty"`
	Actions         []workflowActionBody `json:"actions"`
}

type workflowResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	IsEnabled       bool    `json:"isEnabled"`
	Trigger         string  `json:"trigger"`
	ConditionJSON   *string `json:"conditionJson"`
	CooldownMinutes *int64  `json:"cooldownMinutes"`
	Actions         []struct {
		ActionType string `json:"actionType"`
		ConfigJSON string `json:"configJson"`
	} `json:"actions"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (r *workflowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow"
}

func (r *workflowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *workflowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An alerting workflow: when an event matching `trigger` fires (and the optional " +
			"`condition_json` matches), the `actions` run in order. Requires `alerts.manage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Workflow ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Workflow name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Optional:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the workflow runs. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"trigger": schema.StringAttribute{
				MarkdownDescription: "Event that starts the workflow, for example `alert.fired`, `alert.resolved` " +
					"or `incident.opened`.",
				Required: true,
			},
			"condition_json": schema.StringAttribute{
				MarkdownDescription: "Optional JSON condition evaluated against the event before the actions run. " +
					"Compared as JSON, so formatting differences do not show as drift.",
				Optional: true,
			},
			"cooldown_minutes": schema.Int64Attribute{
				MarkdownDescription: "Minimum minutes between two runs of this workflow. Once set it can be " +
					"changed but not cleared: the API reads an omitted value as unchanged.",
				Optional: true,
			},
			"actions": schema.ListNestedAttribute{
				MarkdownDescription: "Actions run in list order. Always sent whole, so removing one deletes it.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action_type": schema.StringAttribute{
							MarkdownDescription: "One of `Slack`, `Webhook`, `PagerDuty`, `Teams`, `Email`, `TicketReply`.",
							Required:            true,
							Validators: []validator.String{stringvalidator.OneOf(
								"Slack", "Webhook", "PagerDuty", "Teams", "Email", "TicketReply")},
						},
						"config_json": schema.StringAttribute{
							MarkdownDescription: "Action configuration as a JSON object (for example the webhook URL " +
								"or Slack channel). Compared as JSON.",
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

func (m *workflowModel) toBody() *workflowBody {
	actions := make([]workflowActionBody, 0, len(m.Actions))
	for i, a := range m.Actions {
		actions = append(actions, workflowActionBody{
			OrderIndex: i,
			ActionType: a.ActionType.ValueString(),
			ConfigJSON: a.ConfigJSON.ValueString(),
		})
	}
	return &workflowBody{
		Name:            m.Name.ValueString(),
		Description:     strPtrClearing(m.Description),
		IsEnabled:       boolPtr(m.IsEnabled),
		Trigger:         m.Trigger.ValueString(),
		ConditionJSON:   strPtrClearing(m.ConditionJSON),
		CooldownMinutes: int64Ptr(m.CooldownMinutes),
		Actions:         actions,
	}
}

func (m *workflowModel) applyResponse(resp *workflowResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.Trigger = types.StringValue(resp.Trigger)
	m.ConditionJSON = jsonKeepingConfig(m.ConditionJSON, []byte(deref(resp.ConditionJSON)))
	m.CooldownMinutes = int64FromPtr(resp.CooldownMinutes)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)

	if len(resp.Actions) == 0 {
		m.Actions = nil
		return
	}
	actions := make([]workflowActionModel, 0, len(resp.Actions))
	for i, a := range resp.Actions {
		prior := types.StringNull()
		if i < len(m.Actions) {
			prior = m.Actions[i].ConfigJSON
		}
		cfg := jsonKeepingConfig(prior, []byte(a.ConfigJSON))
		if cfg.IsNull() {
			// The API stores "{}" for an empty config; config_json is Required, so
			// keep a known value rather than null.
			cfg = types.StringValue(a.ConfigJSON)
			if !prior.IsNull() && !prior.IsUnknown() {
				cfg = prior
			}
		}
		actions = append(actions, workflowActionModel{ActionType: types.StringValue(a.ActionType), ConfigJSON: cfg})
	}
	m.Actions = actions
}

func (r *workflowResource) path(id string) string {
	if id == "" {
		return r.client.Tenantf("/alerting", "/workflows")
	}
	return r.client.Tenantf("/alerting", "/workflows/%s", id)
}

func (r *workflowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan workflowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got workflowResponse
	if err := r.client.Post(ctx, r.path(""), plan.toBody(), &got); err != nil {
		resp.Diagnostics.AddError("Error creating workflow", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *workflowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state workflowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got workflowResponse
	if err := r.client.Get(ctx, r.path(state.ID.ValueString()), nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading workflow", err.Error())
		return
	}
	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *workflowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan workflowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got workflowResponse
	if err := r.client.Patch(ctx, r.path(plan.ID.ValueString()), plan.toBody(), &got); err != nil {
		resp.Diagnostics.AddError("Error updating workflow", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *workflowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state workflowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path(state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting workflow", err.Error())
	}
}

func (r *workflowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
