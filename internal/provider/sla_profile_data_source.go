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

// sla_profile data source looks up a single SLA profile by name from the tenant
// list (GET /monitoring/.../sla-profiles). Only the scalar profile fields are
// exposed. Reuses slaProfileResponse so JSON names match the resource.

var (
	_ datasource.DataSource              = (*slaProfileDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*slaProfileDataSource)(nil)
)

// NewSLAProfileDataSource is the data source constructor wired into the provider.
func NewSLAProfileDataSource() datasource.DataSource { return &slaProfileDataSource{} }

type slaProfileDataSource struct {
	client *client.Client
}

type slaProfileDataSourceModel struct {
	Name                types.String  `tfsdk:"name"`
	ID                  types.String  `tfsdk:"id"`
	Description         types.String  `tfsdk:"description"`
	TargetUptimePercent types.Float64 `tfsdk:"target_uptime_percent"`
	WindowDays          types.Int64   `tfsdk:"window_days"`
	CreatedAt           types.String  `tfsdk:"created_at"`
}

func (d *slaProfileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sla_profile"
}

func (d *slaProfileDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *slaProfileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single SLA profile by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the SLA profile to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "SLA profile ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Computed:            true,
			},
			"target_uptime_percent": schema.Float64Attribute{
				MarkdownDescription: "Target uptime percentage.",
				Computed:            true,
			},
			"window_days": schema.Int64Attribute{
				MarkdownDescription: "Rolling evaluation window in days.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *slaProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config slaProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []slaProfileResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/sla-profiles"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading SLA profiles", err.Error())
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
		config.TargetUptimePercent = types.Float64Value(p.TargetUptimePercent)
		config.WindowDays = types.Int64Value(p.WindowDays)
		config.CreatedAt = types.StringValue(p.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"SLA profile not found",
		fmt.Sprintf("No SLA profile named %q was found in the tenant.", wanted),
	)
}
