package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// incident_severity_policy is one rung of the internal SEV ladder, owned by
// Alerting.Api IncidentResponseController. The internal severity is the
// engineering response contract (who gets paged, how fast, whether a postmortem
// is owed) and is distinct from the public impact level a status page shows.
//
// The ladder is fixed at Sev1..Sev4 and the API always returns all four rows,
// built-in defaults overlaid with the tenant's stored overrides, so `severity`
// is the identity of the resource and forces replacement rather than being a
// created thing. The id is the severity name.
//
// The three numeric and boolean attributes are Optional+Computed: omit one and
// the tenant keeps whatever was effective before, which is the built-in default
// on a tenant that has never customised the rung. UseStateForUnknown because
// the API never changes them on its own. Their validators mirror
// IncidentSeverityDefaults.Clamp exactly: the server silently clamps out of
// range values, and an applied value that differs from the planned one fails
// the apply with "Provider produced inconsistent result after apply", so it is
// better to reject the value at plan time.
//
// page_escalation_policy_id is a plain Optional, NOT Computed: the PUT applies
// it on every call, and omitting it clears the pager assignment, so null must
// stay null rather than being filled in from the server.
//
// There is no delete endpoint. Destroying this resource drops it from state and
// LEAVES the stored override in place; to go back to the built-in ladder, set
// the attributes to the documented defaults before removing the resource.
//
// An API key driving this needs the settings.manage scope as well as
// alerts.manage: the incident-severity path matches no fragment in
// ApiKeyAuthMiddleware.PathScopeMap, so its writes fall through to the write
// default-deny, and the controller then enforces alerts.manage of its own.

var (
	_ resource.Resource                = (*incidentSeverityPolicyResource)(nil)
	_ resource.ResourceWithConfigure   = (*incidentSeverityPolicyResource)(nil)
	_ resource.ResourceWithImportState = (*incidentSeverityPolicyResource)(nil)
)

// incidentSeverities is IncidentSeverityDefaults.All. The API parses the name
// case-insensitively but echoes this spelling back, so the validator pins it.
var incidentSeverities = []string{"Sev1", "Sev2", "Sev3", "Sev4"}

// NewIncidentSeverityPolicyResource is the resource constructor wired into the provider.
func NewIncidentSeverityPolicyResource() resource.Resource { return &incidentSeverityPolicyResource{} }

type incidentSeverityPolicyResource struct {
	client *client.Client
}

// incidentSeverityPolicyModel maps schema attributes <-> Terraform state.
type incidentSeverityPolicyModel struct {
	ID                     types.String `tfsdk:"id"`
	Severity               types.String `tfsdk:"severity"`
	ResponseTargetMinutes  types.Int64  `tfsdk:"response_target_minutes"`
	RequirePostmortem      types.Bool   `tfsdk:"require_postmortem"`
	PostmortemDueHours     types.Int64  `tfsdk:"postmortem_due_hours"`
	PageEscalationPolicyID types.String `tfsdk:"page_escalation_policy_id"`
}

// --- Wire DTOs (camelCase JSON matching the alerting API) ---

type incidentSeverityPolicyRow struct {
	Severity               string  `json:"severity"`
	ResponseTargetMinutes  int64   `json:"responseTargetMinutes"`
	RequirePostmortem      bool    `json:"requirePostmortem"`
	PostmortemDueHours     int64   `json:"postmortemDueHours"`
	PageEscalationPolicyID *string `json:"pageEscalationPolicyId"`
}

type incidentSeverityPolicyListResponse struct {
	Items []incidentSeverityPolicyRow `json:"items"`
}

type incidentSeverityPolicyBody struct {
	ResponseTargetMinutes  *int64  `json:"responseTargetMinutes,omitempty"`
	RequirePostmortem      *bool   `json:"requirePostmortem,omitempty"`
	PostmortemDueHours     *int64  `json:"postmortemDueHours,omitempty"`
	PageEscalationPolicyID *string `json:"pageEscalationPolicyId,omitempty"`
}

func (r *incidentSeverityPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_severity_policy"
}

func (r *incidentSeverityPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *incidentSeverityPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The response contract for one rung of the internal SEV ladder: how fast an " +
			"incident of that severity must be picked up, who gets paged, and whether it owes a postmortem. " +
			"This is the engineering severity, separate from the public impact level a status page shows. " +
			"The ladder is fixed at `" + strings.Join(incidentSeverities, "`, `") + "`, so this resource " +
			"configures a rung that always exists rather than creating one. Attributes left out keep " +
			"whatever was effective before, which is the built-in default on a rung nobody has customised. " +
			"Destroying the resource leaves the stored override in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Severity policy ID, which is the severity name.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"severity": schema.StringAttribute{
				MarkdownDescription: "Which rung of the ladder this configures: `" +
					strings.Join(incidentSeverities, "`, `") + "`. Changing it replaces the resource.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.OneOf(incidentSeverities...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"response_target_minutes": schema.Int64Attribute{
				MarkdownDescription: "How long an incident of this severity may go unacknowledged before " +
					"the response target is breached, in minutes (1 to 10080). Built-in defaults are 5 for " +
					"`Sev1`, 15 for `Sev2`, 60 for `Sev3` and 240 for `Sev4`.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.Int64{int64validator.Between(1, 7*24*60)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"require_postmortem": schema.BoolAttribute{
				MarkdownDescription: "Whether resolving an incident of this severity opens a postmortem " +
					"obligation, which is then reminded on and escalated. Built-in defaults are `true` for " +
					"`Sev1` and `Sev2`, `false` for `Sev3` and `Sev4`.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"postmortem_due_hours": schema.Int64Attribute{
				MarkdownDescription: "How long after resolution the postmortem is due, in hours (1 to 2160). " +
					"Ignored unless `require_postmortem` is `true`. Built-in defaults are 72 for `Sev1`, " +
					"120 for `Sev2` and 168 for `Sev3` and `Sev4`.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.Int64{int64validator.Between(1, 90*24)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"page_escalation_policy_id": schema.StringAttribute{
				MarkdownDescription: "ID of the escalation policy paged when an incident of this severity " +
					"is declared. The policy must belong to this organization. Omit it to page nobody by " +
					"severity, which is also what removing it from the configuration does.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.LengthBetween(1, 128)},
			},
		},
	}
}

func (m *incidentSeverityPolicyModel) toBody() *incidentSeverityPolicyBody {
	return &incidentSeverityPolicyBody{
		ResponseTargetMinutes:  int64Ptr(m.ResponseTargetMinutes),
		RequirePostmortem:      boolPtr(m.RequirePostmortem),
		PostmortemDueHours:     int64Ptr(m.PostmortemDueHours),
		PageEscalationPolicyID: strPtr(m.PageEscalationPolicyID),
	}
}

// findIncidentSeverityPolicy picks one rung out of the ladder the API returns.
// Case-insensitive because the API parses the severity name that way, so an
// imported "sev1" still finds the "Sev1" row.
func findIncidentSeverityPolicy(rows []incidentSeverityPolicyRow, severity string) *incidentSeverityPolicyRow {
	for i := range rows {
		if strings.EqualFold(strings.TrimSpace(rows[i].Severity), strings.TrimSpace(severity)) {
			return &rows[i]
		}
	}
	return nil
}

// applyRow folds the effective policy into state. The severity keeps its
// CONFIGURED spelling: writing the server's back into a Required attribute
// would fail the apply with "was cty.StringVal(\"sev1\"), but now
// cty.StringVal(\"Sev1\")".
func (m *incidentSeverityPolicyModel) applyRow(row *incidentSeverityPolicyRow) {
	m.ID = types.StringValue(m.Severity.ValueString())
	m.ResponseTargetMinutes = types.Int64Value(row.ResponseTargetMinutes)
	m.RequirePostmortem = types.BoolValue(row.RequirePostmortem)
	m.PostmortemDueHours = types.Int64Value(row.PostmortemDueHours)
	m.PageEscalationPolicyID = strFromPtr(row.PageEscalationPolicyID)
}

// fetchRow reads the whole ladder and returns this rung. The API has no
// single-severity GET; the listing is always four rows.
func (r *incidentSeverityPolicyResource) fetchRow(ctx context.Context, m *incidentSeverityPolicyModel) (*incidentSeverityPolicyRow, error) {
	var list incidentSeverityPolicyListResponse
	url := r.client.Tenantf("/alerting", "/incident-severity-policies")
	if err := r.client.Get(ctx, url, nil, &list); err != nil {
		return nil, err
	}
	return findIncidentSeverityPolicy(list.Items, m.Severity.ValueString()), nil
}

// upsert writes the rung. The response is only a success flag, so every caller
// reads the effective policy back.
func (r *incidentSeverityPolicyResource) upsert(ctx context.Context, m *incidentSeverityPolicyModel) error {
	url := r.client.Tenantf("/alerting", "/incident-severity-policies/%s", m.Severity.ValueString())
	return r.client.Put(ctx, url, m.toBody(), nil)
}

// apply is the shared create/update body: write, then read the effective policy
// back into state.
func (r *incidentSeverityPolicyResource) apply(ctx context.Context, m *incidentSeverityPolicyModel) error {
	if err := r.upsert(ctx, m); err != nil {
		return err
	}
	row, err := r.fetchRow(ctx, m)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("the severity ladder came back without a %s row", m.Severity.ValueString())
	}
	m.applyRow(row)
	return nil
}

func (r *incidentSeverityPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan incidentSeverityPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error creating incident severity policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *incidentSeverityPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state incidentSeverityPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	row, err := r.fetchRow(ctx, &state)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading incident severity policy", err.Error())
		return
	}
	if row == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.applyRow(row)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *incidentSeverityPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan incidentSeverityPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error updating incident severity policy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete drops the rung from state only. The API exposes no delete for a
// severity policy, and the rung itself can never stop existing: what is stored
// stays in force until something writes over it.
func (r *incidentSeverityPolicyResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes the severity name, which is also the id.
func (r *incidentSeverityPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("severity"), req, resp)
}
