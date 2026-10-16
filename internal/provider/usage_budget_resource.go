package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// usage_budget maps onto the Watchpoint Billing API's usage budgets - spend
// alerts that are orthogonal to overage hard-caps.

var (
	_ resource.Resource                = (*usageBudgetResource)(nil)
	_ resource.ResourceWithConfigure   = (*usageBudgetResource)(nil)
	_ resource.ResourceWithImportState = (*usageBudgetResource)(nil)
)

// NewUsageBudgetResource is the resource constructor wired into the provider.
func NewUsageBudgetResource() resource.Resource { return &usageBudgetResource{} }

type usageBudgetResource struct {
	client *client.Client
}

// usageBudgetModel maps schema attributes <-> Terraform state.
type usageBudgetModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	ThresholdCents types.Int64  `tfsdk:"threshold_cents"`
	ProductSlug    types.String `tfsdk:"product_slug"`
	Mode           types.String `tfsdk:"mode"`
	NotifyEmail    types.String `tfsdk:"notify_email"`
	IsEnabled      types.Bool   `tfsdk:"is_enabled"`
	LastBreachedAt types.String `tfsdk:"last_breached_at"`
}

// --- Wire DTOs (camelCase JSON matching the billing API) ---

type usageBudgetUpsertBody struct {
	Name           *string `json:"name,omitempty"`
	ThresholdCents int64   `json:"thresholdCents"`
	ProductSlug    *string `json:"productSlug,omitempty"`
	Mode           *string `json:"mode,omitempty"`
	NotifyEmail    *string `json:"notifyEmail,omitempty"`
	IsEnabled      *bool   `json:"isEnabled,omitempty"`
}

type usageBudgetResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ThresholdCents int64   `json:"thresholdCents"`
	ProductSlug    *string `json:"productSlug"`
	Mode           string  `json:"mode"`
	NotifyEmail    *string `json:"notifyEmail"`
	IsEnabled      bool    `json:"isEnabled"`
	LastBreachedAt *string `json:"lastBreachedAt"`
}

func (r *usageBudgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_usage_budget"
}

func (r *usageBudgetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *usageBudgetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A usage budget that fires a notification when projected or actual spend " +
			"crosses a cents threshold. Independent of overage hard-caps.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Usage budget ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable budget name.",
				Required:            true,
			},
			"threshold_cents": schema.Int64Attribute{
				MarkdownDescription: "Spend threshold in cents. Must be positive.",
				Required:            true,
			},
			"product_slug": schema.StringAttribute{
				MarkdownDescription: "Optional product slug to scope the budget to a single product. Omit for an all-products budget.",
				Optional:            true,
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: "Budget evaluation mode: `forecast` (projected) or `actual`. Defaults to `forecast`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("forecast"),
			},
			"notify_email": schema.StringAttribute{
				MarkdownDescription: "Email address notified when the budget is breached.",
				Optional:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the budget is active. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"last_breached_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the most recent breach (RFC 3339), if any.",
				Computed:            true,
			},
		},
	}
}

func (m *usageBudgetModel) toUpsertBody() *usageBudgetUpsertBody {
	return &usageBudgetUpsertBody{
		Name:           strPtr(m.Name),
		ThresholdCents: m.ThresholdCents.ValueInt64(),
		ProductSlug:    strPtr(m.ProductSlug),
		Mode:           strPtr(m.Mode),
		NotifyEmail:    strPtr(m.NotifyEmail),
		IsEnabled:      boolPtr(m.IsEnabled),
	}
}

func (m *usageBudgetModel) applyResponse(resp *usageBudgetResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.ThresholdCents = types.Int64Value(resp.ThresholdCents)
	m.ProductSlug = strFromPtr(resp.ProductSlug)
	m.Mode = types.StringValue(resp.Mode)
	m.NotifyEmail = strFromPtr(resp.NotifyEmail)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.LastBreachedAt = strFromPtr(resp.LastBreachedAt)
}

func (r *usageBudgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan usageBudgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created usageBudgetResponse
	path := r.client.Tenantf("/billing", "/budgets")
	if err := r.client.Post(ctx, path, plan.toUpsertBody(), &created); err != nil {
		resp.Diagnostics.AddError("Error creating usage budget", err.Error())
		return
	}

	plan.applyResponse(&created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *usageBudgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state usageBudgetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The billing API exposes a list endpoint only (no GET-by-id), so fetch
	// the tenant's budgets and find ours.
	var budgets []usageBudgetResponse
	path := r.client.Tenantf("/billing", "/budgets")
	if err := r.client.Get(ctx, path, nil, &budgets); err != nil {
		resp.Diagnostics.AddError("Error reading usage budgets", err.Error())
		return
	}

	id := state.ID.ValueString()
	for i := range budgets {
		if budgets[i].ID == id {
			state.applyResponse(&budgets[i])
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *usageBudgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan usageBudgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated usageBudgetResponse
	path := r.client.Tenantf("/billing", "/budgets/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, plan.toUpsertBody(), &updated); err != nil {
		resp.Diagnostics.AddError("Error updating usage budget", err.Error())
		return
	}

	plan.applyResponse(&updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *usageBudgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state usageBudgetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/billing", "/budgets/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting usage budget", err.Error())
	}
}

func (r *usageBudgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
