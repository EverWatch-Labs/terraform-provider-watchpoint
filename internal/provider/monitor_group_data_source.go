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

// monitor_group data source looks up a single monitor group by name from the
// tenant group listing (GET /monitoring/.../groups), which is the only endpoint
// that returns the full group record.

var (
	_ datasource.DataSource              = (*monitorGroupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*monitorGroupDataSource)(nil)
)

// NewMonitorGroupDataSource is the data source constructor wired into the provider.
func NewMonitorGroupDataSource() datasource.DataSource { return &monitorGroupDataSource{} }

type monitorGroupDataSource struct {
	client *client.Client
}

// monitorGroupDataSourceModel maps the resolved group into state.
type monitorGroupDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	Color       types.String `tfsdk:"color"`
	Icon        types.String `tfsdk:"icon"`
	SortOrder   types.Int64  `tfsdk:"sort_order"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (d *monitorGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_group"
}

func (d *monitorGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *monitorGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single monitor group by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the monitor group to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Monitor group ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Free-form group description.",
				Computed:            true,
			},
			"color": schema.StringAttribute{
				MarkdownDescription: "Hex color used for the group's UI accent.",
				Computed:            true,
			},
			"icon": schema.StringAttribute{
				MarkdownDescription: "Icon shown for the group in the UI.",
				Computed:            true,
			},
			"sort_order": schema.Int64Attribute{
				MarkdownDescription: "Display sort order.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *monitorGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config monitorGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []monitorGroupResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/groups"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading monitor groups", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		g := &list[i]
		if !strings.EqualFold(g.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(g.ID)
		config.Description = strFromPtr(g.Description)
		config.Color = strFromPtr(g.Color)
		config.Icon = strFromPtr(g.Icon)
		config.SortOrder = types.Int64Value(g.SortOrder)
		config.CreatedAt = types.StringValue(g.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Monitor group not found",
		fmt.Sprintf("No monitor group named %q was found in the tenant.", wanted),
	)
}
