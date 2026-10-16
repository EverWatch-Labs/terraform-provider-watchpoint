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

// alert_routing_rule data source looks up a single routing rule by name from
// the tenant list (GET /alerting/.../alert-routing-rules). Only scalar fields
// are exposed; conditions and targets belong to the resource. Reuses
// alertRoutingRuleResponse.

var (
	_ datasource.DataSource              = (*alertRoutingRuleDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*alertRoutingRuleDataSource)(nil)
)

// NewAlertRoutingRuleDataSource is the data source constructor wired into the provider.
func NewAlertRoutingRuleDataSource() datasource.DataSource { return &alertRoutingRuleDataSource{} }

type alertRoutingRuleDataSource struct {
	client *client.Client
}

type alertRoutingRuleDataSourceModel struct {
	Name           types.String `tfsdk:"name"`
	ID             types.String `tfsdk:"id"`
	Description    types.String `tfsdk:"description"`
	IsEnabled      types.Bool   `tfsdk:"is_enabled"`
	Priority       types.Int64  `tfsdk:"priority"`
	StopProcessing types.Bool   `tfsdk:"stop_processing"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func (d *alertRoutingRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_routing_rule"
}

func (d *alertRoutingRuleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *alertRoutingRuleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single alert routing rule by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the alert routing rule to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Alert routing rule ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the rule is evaluated.",
				Computed:            true,
			},
			"priority": schema.Int64Attribute{
				MarkdownDescription: "Evaluation order, ascending.",
				Computed:            true,
			},
			"stop_processing": schema.BoolAttribute{
				MarkdownDescription: "Whether a match stops lower-priority rules from being evaluated.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *alertRoutingRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config alertRoutingRuleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []alertRoutingRuleResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/alerting", "/alert-routing-rules"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading alert routing rules", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		r := &list[i]
		if !strings.EqualFold(r.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(r.ID)
		config.Description = strFromPtr(r.Description)
		config.IsEnabled = types.BoolValue(r.IsEnabled)
		config.Priority = types.Int64Value(r.Priority)
		config.StopProcessing = types.BoolValue(r.StopProcessing)
		config.CreatedAt = types.StringValue(r.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Alert routing rule not found",
		fmt.Sprintf("No alert routing rule named %q was found in the tenant.", wanted),
	)
}
