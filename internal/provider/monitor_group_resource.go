package provider

import (
	"context"
	"regexp"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// monitorGroupIcons is the curated set of icon names the UI can render for a
// monitor group. Kept in sync with GROUP_ICONS in the ClientPortal GroupsPage;
// an icon outside this set is stored but renders as the default folder icon, so
// the provider validates against it to fail fast at plan time.
var monitorGroupIcons = []string{
	"folder", "server", "globe", "database", "shield", "activity",
	"cloud", "box", "layers", "cpu", "network", "zap", "gauge",
}

// monitorGroupResource manages a collection of monitors with shared
// group-level profile inheritance.
var (
	_ resource.Resource                = (*monitorGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*monitorGroupResource)(nil)
	_ resource.ResourceWithImportState = (*monitorGroupResource)(nil)
)

// NewMonitorGroupResource is the resource constructor wired into the provider.
func NewMonitorGroupResource() resource.Resource { return &monitorGroupResource{} }

type monitorGroupResource struct {
	client *client.Client
}

// monitorGroupModel maps schema attributes <-> Terraform state.
type monitorGroupModel struct {
	ID                           types.String `tfsdk:"id"`
	Name                         types.String `tfsdk:"name"`
	Description                  types.String `tfsdk:"description"`
	Color                        types.String `tfsdk:"color"`
	Icon                         types.String `tfsdk:"icon"`
	SortOrder                    types.Int64  `tfsdk:"sort_order"`
	LocationProfileID            types.String `tfsdk:"location_profile_id"`
	ThresholdProfileID           types.String `tfsdk:"threshold_profile_id"`
	AlertProfileID               types.String `tfsdk:"alert_profile_id"`
	SLAProfileID                 types.String `tfsdk:"sla_profile_id"`
	ExpectedResponseTypes        types.String `tfsdk:"expected_response_types"`
	ExpectedResponseTypeSeverity types.String `tfsdk:"expected_response_type_severity"`
	Tags                         types.Set    `tfsdk:"tags"`
	CreatedAt                    types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type monitorGroupBody struct {
	Name                         *string `json:"name,omitempty"`
	Description                  *string `json:"description,omitempty"`
	Color                        *string `json:"color,omitempty"`
	Icon                         *string `json:"icon,omitempty"`
	SortOrder                    *int64  `json:"sortOrder,omitempty"`
	LocationProfileID            *string `json:"locationProfileId,omitempty"`
	ThresholdProfileID           *string `json:"thresholdProfileId,omitempty"`
	AlertProfileID               *string `json:"alertProfileId,omitempty"`
	SLAProfileID                 *string `json:"slaProfileId,omitempty"`
	ExpectedResponseTypes        *string `json:"expectedResponseTypes,omitempty"`
	ExpectedResponseTypeSeverity *string `json:"expectedResponseTypeSeverity,omitempty"`
	Tags                         *string `json:"tags,omitempty"`
}

// monitorGroupResponse mirrors the tenant-scoped group listing projection,
// which is the only place the full group record (with all fields) is returned.
type monitorGroupResponse struct {
	ID                           string  `json:"id"`
	Name                         string  `json:"name"`
	Description                  *string `json:"description"`
	Color                        *string `json:"color"`
	Icon                         *string `json:"icon"`
	SortOrder                    int64   `json:"sortOrder"`
	LocationProfileID            *string `json:"locationProfileId"`
	ThresholdProfileID           *string `json:"thresholdProfileId"`
	AlertProfileID               *string `json:"alertProfileId"`
	SLAProfileID                 *string `json:"slaProfileId"`
	ExpectedResponseTypes        *string `json:"expectedResponseTypes"`
	ExpectedResponseTypeSeverity string  `json:"expectedResponseTypeSeverity"`
	Tags                         *string `json:"tags"`
	CreatedAt                    string  `json:"createdAt"`
}

func (r *monitorGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_group"
}

func (r *monitorGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *monitorGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A monitor group organizes monitors into a collection with shared group-level " +
			"profile inheritance (location / threshold / alert / SLA). Monitor-level profiles override group-level ones.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Monitor group ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable group name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Free-form group description.",
				Optional:            true,
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Hex color used for the group's UI accent, in `#RRGGBB` form (e.g. `#3b82f6`). " +
					"Any valid 6-digit hex is accepted; the portal's color picker offers these presets: " +
					"`#0f766e`, `#3b82f6`, `#10b981`, `#f59e0b`, `#ef4444`, `#14b8a6`, `#ec4899`, `#f97316`, `#64748b`.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^#[0-9a-fA-F]{6}$`),
						"must be a 6-digit hex color like #3b82f6",
					),
				},
			},
			"icon": schema.StringAttribute{
				MarkdownDescription: "Icon shown for the group in the UI. Must be one of the supported names: " +
					"`folder`, `server`, `globe`, `database`, `shield`, `activity`, `cloud`, `box`, `layers`, `cpu`, `network`, `zap`, `gauge`. " +
					"Any other value is stored but renders as the default `folder` icon.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf(monitorGroupIcons...),
				},
			},
			"sort_order": schema.Int64Attribute{
				MarkdownDescription: "Display sort order. Defaults to `0`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
			},
			"location_profile_id": schema.StringAttribute{
				MarkdownDescription: "Location profile inherited by monitors in this group.",
				Optional:            true,
			},
			"threshold_profile_id": schema.StringAttribute{
				MarkdownDescription: "Threshold profile inherited by monitors in this group.",
				Optional:            true,
			},
			"alert_profile_id": schema.StringAttribute{
				MarkdownDescription: "Alert profile inherited by monitors in this group.",
				Optional:            true,
			},
			"sla_profile_id": schema.StringAttribute{
				MarkdownDescription: "SLA profile inherited by monitors in this group.",
				Optional:            true,
			},
			"expected_response_types": schema.StringAttribute{
				MarkdownDescription: "Comma-separated allowed Content-Types inherited by HTTP monitors in this group.",
				Optional:            true,
			},
			"expected_response_type_severity": schema.StringAttribute{
				MarkdownDescription: "Severity raised on a response-type mismatch. Defaults to `Warning`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("Warning"),
			},
			"tags": schema.SetAttribute{
				MarkdownDescription: "Free-form tags inherited by every monitor in the group. " +
					"Use `key:value` to group cost-allocation spend, or a plain label. " +
					"A tag set on a monitor overrides the group's value for the same key. Unordered.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *monitorGroupModel) toBody(ctx context.Context, diags *diag.Diagnostics) *monitorGroupBody {
	return &monitorGroupBody{
		Name:                         strPtr(m.Name),
		Description:                  strPtr(m.Description),
		Color:                        strPtr(m.Color),
		Icon:                         strPtr(m.Icon),
		SortOrder:                    int64Ptr(m.SortOrder),
		LocationProfileID:            strPtr(m.LocationProfileID),
		ThresholdProfileID:           strPtr(m.ThresholdProfileID),
		AlertProfileID:               strPtr(m.AlertProfileID),
		SLAProfileID:                 strPtr(m.SLAProfileID),
		ExpectedResponseTypes:        strPtr(m.ExpectedResponseTypes),
		ExpectedResponseTypeSeverity: strPtr(m.ExpectedResponseTypeSeverity),
		Tags:                         csvPtrFromSet(ctx, m.Tags, diags),
	}
}

func (m *monitorGroupModel) applyResponse(ctx context.Context, resp *monitorGroupResponse, diags *diag.Diagnostics) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.Color = strFromPtr(resp.Color)
	m.Icon = strFromPtr(resp.Icon)
	m.SortOrder = types.Int64Value(resp.SortOrder)
	m.LocationProfileID = strFromPtr(resp.LocationProfileID)
	m.ThresholdProfileID = strFromPtr(resp.ThresholdProfileID)
	m.AlertProfileID = strFromPtr(resp.AlertProfileID)
	m.SLAProfileID = strFromPtr(resp.SLAProfileID)
	m.ExpectedResponseTypes = strFromPtr(resp.ExpectedResponseTypes)
	m.ExpectedResponseTypeSeverity = types.StringValue(resp.ExpectedResponseTypeSeverity)
	m.Tags = setFromCSVPtr(ctx, resp.Tags, diags)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
}

// findGroupByID fetches the tenant's groups and returns the one matching id.
// The tenant group listing is the only endpoint that returns the full group
// record (the POST/PATCH responses are {id,name} / {message} only). Returns a
// nil pointer with no error when the group is absent so callers can branch on
// "not found".
func (r *monitorGroupResource) findGroupByID(ctx context.Context, id string) (*monitorGroupResponse, error) {
	var groups []monitorGroupResponse
	path := r.client.Tenantf("/monitoring", "/groups")
	if err := r.client.Get(ctx, path, nil, &groups); err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == id {
			return &groups[i], nil
		}
	}
	return nil, nil
}

func (r *monitorGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created idNameResponse
	path := r.client.Tenantf("/monitoring", "/groups")
	if err := r.client.Post(ctx, path, plan.toBody(ctx, &resp.Diagnostics), &created); err != nil {
		resp.Diagnostics.AddError("Error creating monitor group", err.Error())
		return
	}

	got, err := r.findGroupByID(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading monitor group after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Error reading monitor group after create",
			"Created group "+created.ID+" was not found in the tenant group listing.")
		return
	}
	plan.applyResponse(ctx, got, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.findGroupByID(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading monitor group", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(ctx, got, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *monitorGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan monitorGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toBody(ctx, &resp.Diagnostics)
	// An emptied tag set has to reach the API as "" or the old tags survive the PATCH.
	body.Tags = csvPtrClearing(ctx, plan.Tags, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/groups/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating monitor group", err.Error())
		return
	}

	got, err := r.findGroupByID(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading monitor group after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Error reading monitor group after update",
			"Updated group "+plan.ID.ValueString()+" was not found in the tenant group listing.")
		return
	}
	plan.applyResponse(ctx, got, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/groups/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting monitor group", err.Error())
	}
}

func (r *monitorGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
