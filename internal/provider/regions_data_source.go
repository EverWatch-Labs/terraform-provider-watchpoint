package provider

import (
	"context"
	"fmt"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// regions data source exposes the public region catalog. The endpoint is
// non-tenant-scoped (/regions/list) and available to all authenticated users.

var (
	_ datasource.DataSource              = (*regionsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*regionsDataSource)(nil)
)

// NewRegionsDataSource is the data source constructor wired into the provider.
func NewRegionsDataSource() datasource.DataSource { return &regionsDataSource{} }

type regionsDataSource struct {
	client *client.Client
}

// regionModel maps a single region object into state.
type regionModel struct {
	ID               types.String  `tfsdk:"id"`
	Name             types.String  `tfsdk:"name"`
	Slug             types.String  `tfsdk:"slug"`
	DisplayName      types.String  `tfsdk:"display_name"`
	Country          types.String  `tfsdk:"country"`
	CountryCode      types.String  `tfsdk:"country_code"`
	City             types.String  `tfsdk:"city"`
	Provider         types.String  `tfsdk:"provider"`
	ProviderRegion   types.String  `tfsdk:"provider_region"`
	Latitude         types.Float64 `tfsdk:"latitude"`
	Longitude        types.Float64 `tfsdk:"longitude"`
	Tier             types.String  `tfsdk:"tier"`
	IsActive         types.Bool    `tfsdk:"is_active"`
	IsPublic         types.Bool    `tfsdk:"is_public"`
	SortOrder        types.Int64   `tfsdk:"sort_order"`
	ProbeNodeCount   types.Int64   `tfsdk:"probe_node_count"`
	HealthyNodeCount types.Int64   `tfsdk:"healthy_node_count"`
}

type regionsDataSourceModel struct {
	Regions []regionModel `tfsdk:"regions"`
}

// regionResponse mirrors RegionResponse (probes omitted for the public list).
type regionResponse struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Slug             string  `json:"slug"`
	DisplayName      *string `json:"displayName"`
	Country          string  `json:"country"`
	CountryCode      string  `json:"countryCode"`
	City             *string `json:"city"`
	Provider         *string `json:"provider"`
	ProviderRegion   *string `json:"providerRegion"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	Tier             string  `json:"tier"`
	IsActive         bool    `json:"isActive"`
	IsPublic         bool    `json:"isPublic"`
	SortOrder        int64   `json:"sortOrder"`
	ProbeNodeCount   int64   `json:"probeNodeCount"`
	HealthyNodeCount int64   `json:"healthyNodeCount"`
}

func (d *regionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_regions"
}

func (d *regionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *regionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The public catalog of monitoring regions available to authenticated users.",
		Attributes: map[string]schema.Attribute{
			"regions": schema.ListNestedAttribute{
				MarkdownDescription: "All active, public regions.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Region ID."},
						"name":               schema.StringAttribute{Computed: true, MarkdownDescription: "Region name."},
						"slug":               schema.StringAttribute{Computed: true, MarkdownDescription: "Region slug."},
						"display_name":       schema.StringAttribute{Computed: true, MarkdownDescription: "Friendly display name."},
						"country":            schema.StringAttribute{Computed: true, MarkdownDescription: "Country."},
						"country_code":       schema.StringAttribute{Computed: true, MarkdownDescription: "ISO country code."},
						"city":               schema.StringAttribute{Computed: true, MarkdownDescription: "City."},
						"provider":           schema.StringAttribute{Computed: true, MarkdownDescription: "Cloud provider."},
						"provider_region":    schema.StringAttribute{Computed: true, MarkdownDescription: "Provider-native region identifier."},
						"latitude":           schema.Float64Attribute{Computed: true, MarkdownDescription: "Latitude."},
						"longitude":          schema.Float64Attribute{Computed: true, MarkdownDescription: "Longitude."},
						"tier":               schema.StringAttribute{Computed: true, MarkdownDescription: "Region tier (e.g. `Standard`, `Premium`, `Enterprise`)."},
						"is_active":          schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the region is active."},
						"is_public":          schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the region is publicly listed."},
						"sort_order":         schema.Int64Attribute{Computed: true, MarkdownDescription: "Display sort order."},
						"probe_node_count":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Total probe nodes in the region."},
						"healthy_node_count": schema.Int64Attribute{Computed: true, MarkdownDescription: "Healthy, active probe nodes."},
					},
				},
			},
		},
	}
}

func (d *regionsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var got []regionResponse
	if err := d.client.Get(ctx, "/regions/list", nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading regions", err.Error())
		return
	}

	state := regionsDataSourceModel{Regions: make([]regionModel, 0, len(got))}
	for i := range got {
		r := &got[i]
		state.Regions = append(state.Regions, regionModel{
			ID:               types.StringValue(r.ID),
			Name:             types.StringValue(r.Name),
			Slug:             types.StringValue(r.Slug),
			DisplayName:      strFromPtr(r.DisplayName),
			Country:          types.StringValue(r.Country),
			CountryCode:      types.StringValue(r.CountryCode),
			City:             strFromPtr(r.City),
			Provider:         strFromPtr(r.Provider),
			ProviderRegion:   strFromPtr(r.ProviderRegion),
			Latitude:         types.Float64Value(r.Latitude),
			Longitude:        types.Float64Value(r.Longitude),
			Tier:             types.StringValue(r.Tier),
			IsActive:         types.BoolValue(r.IsActive),
			IsPublic:         types.BoolValue(r.IsPublic),
			SortOrder:        types.Int64Value(r.SortOrder),
			ProbeNodeCount:   types.Int64Value(r.ProbeNodeCount),
			HealthyNodeCount: types.Int64Value(r.HealthyNodeCount),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
