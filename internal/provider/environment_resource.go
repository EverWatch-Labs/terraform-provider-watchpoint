package provider

import (
	"context"
	"net/url"
	"regexp"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A monitoring environment (Monitoring, /environments). The first one a tenant
// creates becomes its default; there is no GET by id, so Read lists and selects.

var (
	_ resource.Resource                = (*environmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*environmentResource)(nil)
	_ resource.ResourceWithImportState = (*environmentResource)(nil)
)

var environmentColors = []string{"slate", "green", "amber", "blue", "violet", "rose"}

// NewEnvironmentResource is the resource constructor wired into the provider.
func NewEnvironmentResource() resource.Resource { return &environmentResource{} }

type environmentResource struct {
	client *client.Client
}

type environmentModel struct {
	ID           types.String `tfsdk:"id"`
	Key          types.String `tfsdk:"key"`
	Name         types.String `tfsdk:"name"`
	Color        types.String `tfsdk:"color"`
	IsProduction types.Bool   `tfsdk:"is_production"`
	SortOrder    types.Int64  `tfsdk:"sort_order"`
	IsDefault    types.Bool   `tfsdk:"is_default"`
	ReassignTo   types.String `tfsdk:"reassign_to"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

type environmentBody struct {
	Key          *string `json:"key,omitempty"`
	Name         *string `json:"name,omitempty"`
	Color        *string `json:"color,omitempty"`
	IsProduction *bool   `json:"isProduction,omitempty"`
	SortOrder    *int64  `json:"sortOrder,omitempty"`
}

type environmentResponse struct {
	ID           string `json:"id"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	Color        string `json:"color"`
	SortOrder    int64  `json:"sortOrder"`
	IsProduction bool   `json:"isProduction"`
	IsDefault    bool   `json:"isDefault"`
	MonitorCount int64  `json:"monitorCount"`
	ServiceCount int64  `json:"serviceCount"`
	CreatedAt    string `json:"createdAt"`
}

func (r *environmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *environmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An environment (such as production, staging or development) that monitors, services, " +
			"maintenance windows and reports are scoped to. The first environment a tenant creates becomes its " +
			"default, and every monitor without an `environment_id` belongs to the default. Reads need " +
			"`environments.read`, writes `environments.manage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Environment ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "Stable key: 1 to 32 lowercase letters, digits or hyphens, not `default`. Used by " +
					"alert routing `Environment` conditions. Changing it replaces the environment.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`), "must be 1 to 32 lowercase letters, digits or hyphens"),
					stringvalidator.NoneOf("default"),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name (64 characters max).",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 64)},
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Chip color: `slate`, `green`, `amber`, `blue`, `violet` or `rose`. Defaults to `slate`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("slate"),
				Validators:          []validator.String{stringvalidator.OneOf(environmentColors...)},
			},
			"is_production": schema.BoolAttribute{
				MarkdownDescription: "Marks the environment as production. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"sort_order": schema.Int64Attribute{
				MarkdownDescription: "Position in pickers and filters (0 to 100000). When omitted the server appends it after the others.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64validator.Between(0, 100000)},
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the tenant's default environment. Set `true` to make it the " +
					"default; the previous default is cleared, so set it on one environment only. Only `true` is " +
					"accepted: omit it elsewhere. The first environment created is the default automatically.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Validators:    []validator.Bool{onlyTrue{}},
			},
			"reassign_to": schema.StringAttribute{
				MarkdownDescription: "Environment ID that monitors and services in this environment move to when it is " +
					"destroyed. Required to destroy an environment that is still in use. The default environment " +
					"cannot be destroyed.",
				Optional: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (m *environmentModel) applyResponse(in *environmentResponse) {
	m.ID = types.StringValue(in.ID)
	m.Key = types.StringValue(in.Key)
	m.Name = types.StringValue(in.Name)
	m.Color = types.StringValue(in.Color)
	m.IsProduction = types.BoolValue(in.IsProduction)
	m.SortOrder = types.Int64Value(in.SortOrder)
	m.IsDefault = types.BoolValue(in.IsDefault)
	m.CreatedAt = types.StringValue(in.CreatedAt)
}

func (r *environmentResource) path(suffix string) string {
	return r.client.Tenantf("/monitoring", "/environments%s", suffix)
}

// listEnvironments is shared with the data source.
func listEnvironments(ctx context.Context, c *client.Client) ([]environmentResponse, error) {
	var out struct {
		Items []environmentResponse `json:"items"`
	}
	err := c.Get(ctx, c.Tenantf("/monitoring", "/environments"), nil, &out)
	return out.Items, err
}

func (r *environmentResource) find(ctx context.Context, id string) (*environmentResponse, error) {
	rows, err := listEnvironments(ctx, r.client)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i], nil
		}
	}
	return nil, nil
}

// settle applies is_default and reads back the canonical row.
func (r *environmentResource) settle(ctx context.Context, plan *environmentModel, got *environmentResponse) error {
	if plan.IsDefault.ValueBool() && !got.IsDefault {
		if err := r.client.Put(ctx, r.path("/"+got.ID+"/default"), struct{}{}, got); err != nil {
			return err
		}
	}
	plan.applyResponse(got)
	return nil
}

// onlyTrue rejects is_default = false: a default cannot be unset, only moved.
type onlyTrue struct{}

func (onlyTrue) Description(context.Context) string         { return "must be true or omitted" }
func (onlyTrue) MarkdownDescription(context.Context) string { return "must be `true` or omitted" }
func (onlyTrue) ValidateBool(_ context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && !req.ConfigValue.ValueBool() {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid is_default",
			"is_default can only be set to true. Omit it on environments that are not the default.")
	}
}

func (r *environmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got environmentResponse
	err := r.client.Post(ctx, r.path(""), &environmentBody{
		Key: strPtr(plan.Key), Name: strPtr(plan.Name), Color: strPtr(plan.Color), IsProduction: boolPtr(plan.IsProduction),
	}, &got)
	if err != nil {
		resp.Diagnostics.AddError("Error creating environment", err.Error())
		return
	}
	// POST has no sortOrder; a configured one goes through PATCH.
	if so := int64Ptr(plan.SortOrder); so != nil && *so != got.SortOrder {
		if err := r.client.Patch(ctx, r.path("/"+got.ID), &environmentBody{SortOrder: so}, &got); err != nil {
			resp.Diagnostics.AddError("Error setting environment sort order", err.Error())
			return
		}
	}
	if err := r.settle(ctx, &plan, &got); err != nil {
		resp.Diagnostics.AddError("Error making environment the default", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *environmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.find(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading environment", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *environmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got environmentResponse
	err := r.client.Patch(ctx, r.path("/"+plan.ID.ValueString()), &environmentBody{
		Name: strPtr(plan.Name), Color: strPtr(plan.Color), IsProduction: boolPtr(plan.IsProduction), SortOrder: int64Ptr(plan.SortOrder),
	}, &got)
	if err != nil {
		resp.Diagnostics.AddError("Error updating environment", err.Error())
		return
	}
	if err := r.settle(ctx, &plan, &got); err != nil {
		resp.Diagnostics.AddError("Error making environment the default", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *environmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var q url.Values
	if to := state.ReassignTo.ValueString(); to != "" {
		q = url.Values{"reassignTo": {to}}
	}
	err := r.client.Do(ctx, "DELETE", r.path("/"+state.ID.ValueString()), q, nil, nil)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting environment", err.Error())
	}
}

func (r *environmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
