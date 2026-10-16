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

// maintenance_window data source looks up a single maintenance window by name
// from the tenant list (GET /monitoring/.../maintenance-windows). Reuses
// maintenanceWindowResponse. starts_at/ends_at mirror the resource's UTC times.

var (
	_ datasource.DataSource              = (*maintenanceWindowDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*maintenanceWindowDataSource)(nil)
)

// NewMaintenanceWindowDataSource is the data source constructor wired into the provider.
func NewMaintenanceWindowDataSource() datasource.DataSource { return &maintenanceWindowDataSource{} }

type maintenanceWindowDataSource struct {
	client *client.Client
}

type maintenanceWindowDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	StartsAt    types.String `tfsdk:"starts_at"`
	EndsAt      types.String `tfsdk:"ends_at"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (d *maintenanceWindowDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

func (d *maintenanceWindowDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *maintenanceWindowDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single maintenance window by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the maintenance window to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Maintenance window ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Computed:            true,
			},
			"starts_at": schema.StringAttribute{
				MarkdownDescription: "Window start (RFC 3339, UTC).",
				Computed:            true,
			},
			"ends_at": schema.StringAttribute{
				MarkdownDescription: "Window end (RFC 3339, UTC).",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *maintenanceWindowDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config maintenanceWindowDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []maintenanceWindowResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/maintenance-windows"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading maintenance windows", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.Description = strFromPtr(p.Description)
		config.StartsAt = types.StringValue(p.StartTimeUTC)
		config.EndsAt = types.StringValue(p.EndTimeUTC)
		config.CreatedAt = types.StringValue(p.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Maintenance window not found",
		fmt.Sprintf("No maintenance window named %q was found in the tenant.", wanted),
	)
}
