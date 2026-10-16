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

// oncall_schedule data source looks up a single on-call schedule by name from the
// tenant list (GET /monitoring/.../oncall-schedules). Only scalar fields are
// exposed; participants are omitted. Reuses onCallScheduleResponse.

var (
	_ datasource.DataSource              = (*onCallScheduleDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*onCallScheduleDataSource)(nil)
)

// NewOnCallScheduleDataSource is the data source constructor wired into the provider.
func NewOnCallScheduleDataSource() datasource.DataSource { return &onCallScheduleDataSource{} }

type onCallScheduleDataSource struct {
	client *client.Client
}

type onCallScheduleDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	Timezone    types.String `tfsdk:"timezone"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (d *onCallScheduleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_schedule"
}

func (d *onCallScheduleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *onCallScheduleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single on-call schedule by name within the tenant. " +
			"Returns the scalar schedule fields only; participants are not included.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the on-call schedule to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "On-call schedule ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Computed:            true,
			},
			"timezone": schema.StringAttribute{
				MarkdownDescription: "Rotation anchor timestamp (RFC 3339) the schedule is anchored to.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *onCallScheduleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config onCallScheduleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []onCallScheduleResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/alerting", "/oncall-schedules"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading on-call schedules", err.Error())
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
		config.Timezone = types.StringValue(p.AnchorUTC)
		config.CreatedAt = types.StringValue(p.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"On-call schedule not found",
		fmt.Sprintf("No on-call schedule named %q was found in the tenant.", wanted),
	)
}
