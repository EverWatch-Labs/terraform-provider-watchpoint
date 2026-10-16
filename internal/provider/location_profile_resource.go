package provider

import (
	"context"
	"fmt"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Location profiles control which probe regions run a monitor's checks. The
// monitoring API exposes list/create/patch/delete but no GET-by-id, so reads
// resolve the row out of the tenant list.

var (
	_ resource.Resource                = (*locationProfileResource)(nil)
	_ resource.ResourceWithConfigure   = (*locationProfileResource)(nil)
	_ resource.ResourceWithImportState = (*locationProfileResource)(nil)
)

// NewLocationProfileResource is the resource constructor wired into the provider.
func NewLocationProfileResource() resource.Resource { return &locationProfileResource{} }

type locationProfileResource struct {
	client *client.Client
}

// locationProfileModel maps schema attributes <-> Terraform state.
type locationProfileModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	IsDefault types.Bool   `tfsdk:"is_default"`
	IsGlobal  types.Bool   `tfsdk:"is_global"`
	RegionIDs types.Set    `tfsdk:"region_ids"`
	CreatedAt types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type locationProfileCreateBody struct {
	Name      string   `json:"name"`
	IsDefault *bool    `json:"isDefault,omitempty"`
	RegionIDs []string `json:"regionIds,omitempty"`
}

// locationProfileUpdateBody mirrors the PATCH DTO; pointers omit unchanged fields.
type locationProfileUpdateBody = locationProfileCreateBody

type locationProfileRegionResponse struct {
	ID         string `json:"id"`
	RegionID   string `json:"regionId"`
	RegionName string `json:"regionName"`
}

type locationProfileResponse struct {
	ID        string                          `json:"id"`
	Name      string                          `json:"name"`
	IsDefault bool                            `json:"isDefault"`
	IsGlobal  bool                            `json:"isGlobal"`
	CreatedAt string                          `json:"createdAt"`
	Regions   []locationProfileRegionResponse `json:"regions"`
}

func (r *locationProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_location_profile"
}

func (r *locationProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *locationProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A location profile selecting which probe regions execute a monitor's checks.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Location profile ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable profile name.",
				Required:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the tenant's default location profile. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"is_global": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a global, system-managed profile (read-only).",
				Computed:            true,
			},
			"region_ids": schema.SetAttribute{
				MarkdownDescription: "Region IDs to run checks from. Unordered; the order in configuration does not matter.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *locationProfileModel) toCreateBody(ctx context.Context, diags *diagSink) *locationProfileCreateBody {
	return &locationProfileCreateBody{
		Name:      m.Name.ValueString(),
		IsDefault: boolPtr(m.IsDefault),
		RegionIDs: stringSliceFromSet(ctx, m.RegionIDs, diags.d),
	}
}

// applyResponse copies API response fields into the model.
func (m *locationProfileModel) applyResponse(ctx context.Context, resp *locationProfileResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.IsDefault = types.BoolValue(resp.IsDefault)
	m.IsGlobal = types.BoolValue(resp.IsGlobal)
	m.CreatedAt = types.StringValue(resp.CreatedAt)

	regionIDs := make([]string, 0, len(resp.Regions))
	for _, rg := range resp.Regions {
		regionIDs = append(regionIDs, rg.RegionID)
	}
	if len(regionIDs) == 0 {
		m.RegionIDs = types.SetNull(types.StringType)
	} else {
		m.RegionIDs = setFromStringSlice(ctx, regionIDs, diags.d)
	}
}

// findLocationProfile resolves a single profile out of the tenant list, since
// the monitoring API offers no GET-by-id for location profiles.
func (r *locationProfileResource) findLocationProfile(ctx context.Context, id string) (*locationProfileResponse, error) {
	var list []locationProfileResponse
	path := r.client.Tenantf("/monitoring", "/location-profiles")
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

func (r *locationProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan locationProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toCreateBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var created idNameResponse
	path := r.client.Tenantf("/monitoring", "/location-profiles")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating location profile", err.Error())
		return
	}

	got, err := r.findLocationProfile(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading location profile after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Error reading location profile after create",
			fmt.Sprintf("Location profile %s not found in tenant list after create.", created.ID))
		return
	}
	plan.applyResponse(ctx, got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *locationProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state locationProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.findLocationProfile(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading location profile", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *locationProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan locationProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toCreateBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/location-profiles/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating location profile", err.Error())
		return
	}

	got, err := r.findLocationProfile(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading location profile after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Error reading location profile after update",
			fmt.Sprintf("Location profile %s not found in tenant list after update.", plan.ID.ValueString()))
		return
	}
	plan.applyResponse(ctx, got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *locationProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state locationProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/location-profiles/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting location profile", err.Error())
	}
}

func (r *locationProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
