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

// scheduled_report data source looks up a single scheduled report by name from
// the tenant list (GET /monitoring/.../reports). Reuses scheduledReportResponse.
// is_enabled is derived from the delivery schedule (any non-None schedule is an
// enabled, recurring report); report_kind reflects the report's delivery cadence.

var (
	_ datasource.DataSource              = (*scheduledReportDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*scheduledReportDataSource)(nil)
)

// NewScheduledReportDataSource is the data source constructor wired into the provider.
func NewScheduledReportDataSource() datasource.DataSource { return &scheduledReportDataSource{} }

type scheduledReportDataSource struct {
	client *client.Client
}

type scheduledReportDataSourceModel struct {
	Name       types.String `tfsdk:"name"`
	ID         types.String `tfsdk:"id"`
	ReportKind types.String `tfsdk:"report_kind"`
	IsEnabled  types.Bool   `tfsdk:"is_enabled"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

func (d *scheduledReportDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scheduled_report"
}

func (d *scheduledReportDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *scheduledReportDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single scheduled report by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the scheduled report to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Report ID.",
				Computed:            true,
			},
			"report_kind": schema.StringAttribute{
				MarkdownDescription: "Delivery cadence of the report: `None`, `Daily`, `Weekly`, or `Monthly`.",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the report is on a recurring schedule (true unless the cadence is `None`).",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *scheduledReportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config scheduledReportDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []scheduledReportResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/analytics", "/reports"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading scheduled reports", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.ReportKind = types.StringValue(p.Schedule)
		config.IsEnabled = types.BoolValue(!strings.EqualFold(p.Schedule, "None"))
		config.CreatedAt = types.StringValue(p.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Scheduled report not found",
		fmt.Sprintf("No scheduled report named %q was found in the tenant.", wanted),
	)
}
