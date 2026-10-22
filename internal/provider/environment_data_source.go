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

// Looks up one environment by key from the environments list.

var (
	_ datasource.DataSource              = (*environmentDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*environmentDataSource)(nil)
)

// NewEnvironmentDataSource is the data source constructor wired into the provider.
func NewEnvironmentDataSource() datasource.DataSource { return &environmentDataSource{} }

type environmentDataSource struct {
	client *client.Client
}

type environmentDataSourceModel struct {
	Key          types.String `tfsdk:"key"`
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Color        types.String `tfsdk:"color"`
	IsProduction types.Bool   `tfsdk:"is_production"`
	IsDefault    types.Bool   `tfsdk:"is_default"`
	SortOrder    types.Int64  `tfsdk:"sort_order"`
	MonitorCount types.Int64  `tfsdk:"monitor_count"`
	ServiceCount types.Int64  `tfsdk:"service_count"`
}

func (d *environmentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (d *environmentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *environmentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an environment by key, for example to place monitors in an environment " +
			"managed outside this configuration. Pass `default` to get the tenant's default environment.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				MarkdownDescription: "Environment key (case-insensitive), or `default` for the default environment.",
				Required:            true,
			},
			"id":            schema.StringAttribute{MarkdownDescription: "Environment ID.", Computed: true},
			"name":          schema.StringAttribute{MarkdownDescription: "Display name.", Computed: true},
			"color":         schema.StringAttribute{MarkdownDescription: "Chip color.", Computed: true},
			"is_production": schema.BoolAttribute{MarkdownDescription: "Whether it is marked production.", Computed: true},
			"is_default":    schema.BoolAttribute{MarkdownDescription: "Whether it is the tenant's default.", Computed: true},
			"sort_order":    schema.Int64Attribute{MarkdownDescription: "Position in pickers and filters.", Computed: true},
			"monitor_count": schema.Int64Attribute{MarkdownDescription: "Live monitors in the environment.", Computed: true},
			"service_count": schema.Int64Attribute{MarkdownDescription: "Services in the environment.", Computed: true},
		},
	}
}

func (d *environmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config environmentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rows, err := listEnvironments(ctx, d.client)
	if err != nil {
		resp.Diagnostics.AddError("Error reading environments", err.Error())
		return
	}
	wanted := config.Key.ValueString()
	for i := range rows {
		e := &rows[i]
		if !strings.EqualFold(e.Key, wanted) && !(strings.EqualFold(wanted, "default") && e.IsDefault) {
			continue
		}
		config.ID = types.StringValue(e.ID)
		config.Name = types.StringValue(e.Name)
		config.Color = types.StringValue(e.Color)
		config.IsProduction = types.BoolValue(e.IsProduction)
		config.IsDefault = types.BoolValue(e.IsDefault)
		config.SortOrder = types.Int64Value(e.SortOrder)
		config.MonitorCount = types.Int64Value(e.MonitorCount)
		config.ServiceCount = types.Int64Value(e.ServiceCount)
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}
	resp.Diagnostics.AddError("Environment not found", fmt.Sprintf("No environment with key %q was found in the tenant.", wanted))
}
