package provider

import (
	"context"
	"fmt"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Threshold profiles define latency/error thresholds (warning + critical) per
// metric, plus consecutive-breach logic. The monitoring API exposes
// list/create/patch/delete but no GET-by-id, so reads resolve the row out of
// the tenant list. Metric/Operator are .NET enums (PascalCase strings).

var (
	_ resource.Resource                = (*thresholdProfileResource)(nil)
	_ resource.ResourceWithConfigure   = (*thresholdProfileResource)(nil)
	_ resource.ResourceWithImportState = (*thresholdProfileResource)(nil)
)

// NewThresholdProfileResource is the resource constructor wired into the provider.
func NewThresholdProfileResource() resource.Resource { return &thresholdProfileResource{} }

type thresholdProfileResource struct {
	client *client.Client
}

// thresholdProfileModel maps schema attributes <-> Terraform state.
type thresholdProfileModel struct {
	ID        types.String                `tfsdk:"id"`
	Name      types.String                `tfsdk:"name"`
	IsDefault types.Bool                  `tfsdk:"is_default"`
	IsGlobal  types.Bool                  `tfsdk:"is_global"`
	Rules     []thresholdProfileRuleModel `tfsdk:"rules"`
	CreatedAt types.String                `tfsdk:"created_at"`
}

type thresholdProfileRuleModel struct {
	Metric              types.String  `tfsdk:"metric"`
	Operator            types.String  `tfsdk:"operator"`
	WarningValue        types.Float64 `tfsdk:"warning_value"`
	CriticalValue       types.Float64 `tfsdk:"critical_value"`
	ConsecutiveBreaches types.Int64   `tfsdk:"consecutive_breaches"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type thresholdProfileRuleBody struct {
	Metric              string  `json:"metric"`
	Operator            string  `json:"operator"`
	WarningValue        float64 `json:"warningValue"`
	CriticalValue       float64 `json:"criticalValue"`
	ConsecutiveBreaches int64   `json:"consecutiveBreaches"`
}

type thresholdProfileCreateBody struct {
	Name      string                     `json:"name"`
	IsDefault *bool                      `json:"isDefault,omitempty"`
	Rules     []thresholdProfileRuleBody `json:"rules,omitempty"`
}

// thresholdProfileUpdateBody mirrors the PATCH DTO.
type thresholdProfileUpdateBody = thresholdProfileCreateBody

type thresholdProfileRuleResponse struct {
	ID                  string  `json:"id"`
	Metric              string  `json:"metric"`
	Operator            string  `json:"operator"`
	WarningValue        float64 `json:"warningValue"`
	CriticalValue       float64 `json:"criticalValue"`
	ConsecutiveBreaches int64   `json:"consecutiveBreaches"`
}

type thresholdProfileResponse struct {
	ID        string                         `json:"id"`
	Name      string                         `json:"name"`
	IsDefault bool                           `json:"isDefault"`
	IsGlobal  bool                           `json:"isGlobal"`
	CreatedAt string                         `json:"createdAt"`
	Rules     []thresholdProfileRuleResponse `json:"rules"`
}

func (r *thresholdProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_threshold_profile"
}

func (r *thresholdProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *thresholdProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A threshold profile defining per-metric warning/critical thresholds and " +
			"consecutive-breach logic applied to monitor check results.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Threshold profile ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable profile name.",
				Required:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the tenant's default threshold profile. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"is_global": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a global, system-managed profile (read-only).",
				Computed:            true,
			},
			"rules": schema.ListNestedAttribute{
				MarkdownDescription: "Threshold rules evaluated against check results.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"metric": schema.StringAttribute{
							MarkdownDescription: "Metric to evaluate. One of `ResponseTimeMs`, `DnsLookupMs`, " +
								"`TlsHandshakeMs`, `TimeToFirstByteMs`, `TotalTimeMs`, `StatusCode`, " +
								"`BodySizeBytes`, `SslDaysToExpiry`, `SyntheticTotalTimeMs`, " +
								"`SyntheticStepCount`, `ErrorRate`, `PacketLoss`, `DnsResolutionMs`, " +
								"`TcpConnectTimeMs`, `PingLatencyMs`, `PingPacketLossPercent`.",
							Required: true,
						},
						"operator": schema.StringAttribute{
							MarkdownDescription: "Comparison operator. One of `GreaterThan`, `LessThan`, `Equals`, `NotEquals`.",
							Required:            true,
						},
						"warning_value": schema.Float64Attribute{
							MarkdownDescription: "Value at which a warning is raised.",
							Required:            true,
						},
						"critical_value": schema.Float64Attribute{
							MarkdownDescription: "Value at which a critical alert is raised.",
							Required:            true,
						},
						"consecutive_breaches": schema.Int64Attribute{
							MarkdownDescription: "Consecutive breaching checks required before alerting. Defaults to `1`.",
							Optional:            true,
							Computed:            true,
							Default:             int64default.StaticInt64(1),
						},
					},
				},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *thresholdProfileModel) toCreateBody() *thresholdProfileCreateBody {
	body := &thresholdProfileCreateBody{
		Name:      m.Name.ValueString(),
		IsDefault: boolPtr(m.IsDefault),
	}
	if m.Rules != nil {
		body.Rules = make([]thresholdProfileRuleBody, 0, len(m.Rules))
		for _, rule := range m.Rules {
			cb := int64(1)
			if !rule.ConsecutiveBreaches.IsNull() && !rule.ConsecutiveBreaches.IsUnknown() {
				cb = rule.ConsecutiveBreaches.ValueInt64()
			}
			body.Rules = append(body.Rules, thresholdProfileRuleBody{
				Metric:              rule.Metric.ValueString(),
				Operator:            rule.Operator.ValueString(),
				WarningValue:        rule.WarningValue.ValueFloat64(),
				CriticalValue:       rule.CriticalValue.ValueFloat64(),
				ConsecutiveBreaches: cb,
			})
		}
	}
	return body
}

// applyResponse copies API response fields into the model.
func (m *thresholdProfileModel) applyResponse(resp *thresholdProfileResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.IsDefault = types.BoolValue(resp.IsDefault)
	m.IsGlobal = types.BoolValue(resp.IsGlobal)
	m.CreatedAt = types.StringValue(resp.CreatedAt)

	if resp.Rules == nil {
		m.Rules = nil
		return
	}
	rules := make([]thresholdProfileRuleModel, 0, len(resp.Rules))
	for _, rule := range resp.Rules {
		rules = append(rules, thresholdProfileRuleModel{
			Metric:              types.StringValue(rule.Metric),
			Operator:            types.StringValue(rule.Operator),
			WarningValue:        types.Float64Value(rule.WarningValue),
			CriticalValue:       types.Float64Value(rule.CriticalValue),
			ConsecutiveBreaches: types.Int64Value(rule.ConsecutiveBreaches),
		})
	}
	m.Rules = rules
}

// findThresholdProfile resolves a single profile out of the tenant list, since
// the monitoring API offers no GET-by-id for threshold profiles.
func (r *thresholdProfileResource) findThresholdProfile(ctx context.Context, id string) (*thresholdProfileResponse, error) {
	var list []thresholdProfileResponse
	path := r.client.Tenantf("/monitoring", "/threshold-profiles")
	if err := r.client.Get(ctx, path, nil, &list); err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, nil
}

func (r *thresholdProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan thresholdProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toCreateBody()

	var created idNameResponse
	path := r.client.Tenantf("/monitoring", "/threshold-profiles")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating threshold profile", err.Error())
		return
	}

	got, err := r.findThresholdProfile(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading threshold profile after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Error reading threshold profile after create",
			fmt.Sprintf("Threshold profile %s not found in tenant list after create.", created.ID))
		return
	}
	plan.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *thresholdProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state thresholdProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.findThresholdProfile(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading threshold profile", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *thresholdProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan thresholdProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toCreateBody()

	path := r.client.Tenantf("/monitoring", "/threshold-profiles/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating threshold profile", err.Error())
		return
	}

	got, err := r.findThresholdProfile(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading threshold profile after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Error reading threshold profile after update",
			fmt.Sprintf("Threshold profile %s not found in tenant list after update.", plan.ID.ValueString()))
		return
	}
	plan.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *thresholdProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state thresholdProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/threshold-profiles/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting threshold profile", err.Error())
	}
}

func (r *thresholdProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
