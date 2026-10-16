package provider

import (
	"context"
	"fmt"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// usage_anomaly_alert is the relative sibling of watchpoint_usage_budget. A
// budget is an absolute number someone has to guess in advance; this compares
// each metered product's current run rate against that tenant's OWN trailing
// baseline, so it also catches the tenant whose normal spend already sits above
// every threshold they would have set. Both deliver through the same billing
// recipients and the same notification bell.
//
// A per-tenant SINGLETON on the Billing API (GET/PUT .../usage-anomaly), not a
// per-product rule: the detector sweeps every metered product the tenant reports
// under one enable and one sensitivity. Declare it at most once. Create and
// Update are the same PUT; Delete turns the alerts OFF rather than being a
// no-op, because leaving a destroyed alert resource still paging people is the
// surprise, and disabled is a real state the API models.
//
// sensitivity is validated in the provider to the same [1.5, 6] window the API
// clamps to. Without that validator a config of 10 would be stored as 6 and the
// apply would fail with "Provider produced inconsistent result after apply".
//
// The API also returns a `recent` list of the last few spikes. That is a report,
// not configuration: it is deliberately not modelled, not least because the PUT
// answers with an empty list while the GET fills it, so as a Computed attribute
// it would flip on every apply and flip back on every refresh.
//
// The id is the tenant id, which is what the provider is already scoped to.

var (
	_ resource.Resource                = (*usageAnomalyAlertResource)(nil)
	_ resource.ResourceWithConfigure   = (*usageAnomalyAlertResource)(nil)
	_ resource.ResourceWithImportState = (*usageAnomalyAlertResource)(nil)
)

// Sensitivity window enforced by the billing API (UsageBudgetsController
// MinSensitivity/MaxSensitivity). Below 1.5 an ordinary weekly peak clears the
// threshold; above 6 nothing ever fires.
const (
	usageAnomalyMinSensitivity = 1.5
	usageAnomalyMaxSensitivity = 6.0
)

// NewUsageAnomalyAlertResource is the resource constructor wired into the provider.
func NewUsageAnomalyAlertResource() resource.Resource { return &usageAnomalyAlertResource{} }

type usageAnomalyAlertResource struct {
	client *client.Client
}

// usageAnomalyAlertModel maps schema attributes <-> Terraform state.
type usageAnomalyAlertModel struct {
	ID          types.String  `tfsdk:"id"`
	IsEnabled   types.Bool    `tfsdk:"is_enabled"`
	Sensitivity types.Float64 `tfsdk:"sensitivity"`
}

// --- Wire DTOs (camelCase JSON matching the billing API) ---

// usageAnomalyUpsertBody is the PUT body. Both fields are pointers because the
// API reads an omitted field as "leave unchanged"; this resource always sends
// both (each attribute carries a Default, so neither is ever null) so that a
// tenant with a row already stored is genuinely overwritten rather than adopted.
type usageAnomalyUpsertBody struct {
	IsEnabled   *bool    `json:"isEnabled,omitempty"`
	Sensitivity *float64 `json:"sensitivity,omitempty"`
}

type usageAnomalyAlertResponse struct {
	IsEnabled   bool    `json:"isEnabled"`
	Sensitivity float64 `json:"sensitivity"`
}

func (r *usageAnomalyAlertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_usage_anomaly_alert"
}

func (r *usageAnomalyAlertResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *usageAnomalyAlertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Usage anomaly alerts, which notify the tenant's billing recipients when a " +
			"metered product's run rate rises above that tenant's own trailing baseline. The relative " +
			"counterpart to `watchpoint_usage_budget`, which alerts on an absolute spend threshold. A " +
			"per-tenant singleton covering every metered product: declare it at most once. Destroying it " +
			"turns the alerts off.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tenant ID, which is what this per-tenant setting is keyed on.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether anomaly alerts are evaluated. Defaults to `true`, since " +
					"declaring the resource is the opt-in. The platform default for a tenant that has " +
					"never configured it is off.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"sensitivity": schema.Float64Attribute{
				MarkdownDescription: fmt.Sprintf(
					"Standard deviations above the tenant's own baseline before a run rate counts as a "+
						"spike, between `%.1f` and `%.1f`. LOWER is more sensitive: `%.1f` alerts on "+
						"modest rises and is noisier, `%.1f` alerts only on a dramatic one. Defaults to "+
						"`3.0`, roughly a once-in-a-few-hundred-hours reading for a well-behaved series.",
					usageAnomalyMinSensitivity, usageAnomalyMaxSensitivity,
					usageAnomalyMinSensitivity, usageAnomalyMaxSensitivity),
				Optional: true,
				Computed: true,
				Default:  float64default.StaticFloat64(3.0),
				Validators: []validator.Float64{
					float64validator.Between(usageAnomalyMinSensitivity, usageAnomalyMaxSensitivity),
				},
			},
		},
	}
}

func (m *usageAnomalyAlertModel) toUpsertBody() *usageAnomalyUpsertBody {
	return &usageAnomalyUpsertBody{
		IsEnabled:   boolPtr(m.IsEnabled),
		Sensitivity: float64Ptr(m.Sensitivity),
	}
}

// usageAnomalyDisableBody is what Delete sends. It carries isEnabled only:
// sending a sensitivity alongside it would rewrite a figure the tenant may have
// tuned in the portal, for no reason, on the way out.
func usageAnomalyDisableBody() *usageAnomalyUpsertBody {
	off := false
	return &usageAnomalyUpsertBody{IsEnabled: &off}
}

// applyResponse folds the API's answer into state. The server values are taken
// verbatim so a setting changed in the portal reads back as real drift.
func (m *usageAnomalyAlertModel) applyResponse(tenantID string, resp *usageAnomalyAlertResponse) {
	m.ID = types.StringValue(tenantID)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.Sensitivity = types.Float64Value(resp.Sensitivity)
}

func (r *usageAnomalyAlertResource) anomalyPath() string {
	return r.client.Tenantf("/billing", "/usage-anomaly")
}

func (r *usageAnomalyAlertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan usageAnomalyAlertModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated usageAnomalyAlertResponse
	if err := r.client.Put(ctx, r.anomalyPath(), plan.toUpsertBody(), &updated); err != nil {
		resp.Diagnostics.AddError("Error creating usage anomaly alert", err.Error())
		return
	}

	plan.applyResponse(r.client.TenantID(), &updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *usageAnomalyAlertResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state usageAnomalyAlertModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current usageAnomalyAlertResponse
	if err := r.client.Get(ctx, r.anomalyPath(), nil, &current); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading usage anomaly alert", err.Error())
		return
	}

	state.applyResponse(r.client.TenantID(), &current)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *usageAnomalyAlertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan usageAnomalyAlertModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated usageAnomalyAlertResponse
	if err := r.client.Put(ctx, r.anomalyPath(), plan.toUpsertBody(), &updated); err != nil {
		resp.Diagnostics.AddError("Error updating usage anomaly alert", err.Error())
		return
	}

	plan.applyResponse(r.client.TenantID(), &updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete disables the alerts rather than deleting a row. The singleton always
// exists server-side, and a destroyed alert resource that carries on paging
// people is the behaviour nobody expects.
func (r *usageAnomalyAlertResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.Put(ctx, r.anomalyPath(), usageAnomalyDisableBody(), nil); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting usage anomaly alert", err.Error())
	}
}

// ImportState accepts the tenant id. The value is cosmetic: Read re-derives the
// id from the client's own tenant scope, so any id imports the one row.
func (r *usageAnomalyAlertResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
