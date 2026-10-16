package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// location_profile data source looks up a single location profile by name from
// the tenant list (GET /monitoring/.../location-profiles), since the monitoring
// API offers no GET-by-id for location profiles.

var (
	_ datasource.DataSource              = (*locationProfileDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*locationProfileDataSource)(nil)
)

// NewLocationProfileDataSource is the data source constructor wired into the provider.
func NewLocationProfileDataSource() datasource.DataSource { return &locationProfileDataSource{} }

type locationProfileDataSource struct {
	client *client.Client
}

// locationProfileDataSourceModel maps the resolved profile into state.
type locationProfileDataSourceModel struct {
	Name      types.String `tfsdk:"name"`
	ID        types.String `tfsdk:"id"`
	IsDefault types.Bool   `tfsdk:"is_default"`
	IsGlobal  types.Bool   `tfsdk:"is_global"`
	RegionIDs types.Set    `tfsdk:"region_ids"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (d *locationProfileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_location_profile"
}

func (d *locationProfileDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *locationProfileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single location profile by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the location profile to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Location profile ID.",
				Computed:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the tenant's default location profile.",
				Computed:            true,
			},
			"is_global": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a global, system-managed profile.",
				Computed:            true,
			},
			"region_ids": schema.SetAttribute{
				MarkdownDescription: "Region IDs the profile runs checks from.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *locationProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config locationProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []locationProfileResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/location-profiles"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading location profiles", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		lp := &list[i]
		if !strings.EqualFold(lp.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(lp.ID)
		config.IsDefault = types.BoolValue(lp.IsDefault)
		config.IsGlobal = types.BoolValue(lp.IsGlobal)
		config.CreatedAt = types.StringValue(lp.CreatedAt)

		regionIDs := make([]string, 0, len(lp.Regions))
		for _, rg := range lp.Regions {
			regionIDs = append(regionIDs, rg.RegionID)
		}
		set, diags := types.SetValueFrom(ctx, types.StringType, regionIDs)
		resp.Diagnostics.Append(diags...)
		config.RegionIDs = set

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Location profile not found",
		fmt.Sprintf("No location profile named %q was found in the tenant.", wanted),
	)
}
