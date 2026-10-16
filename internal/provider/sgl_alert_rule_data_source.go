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

// sgl_alert_rule data source looks up a single SGL alert rule by name from the
// tenant list (GET /monitoring/.../sgl-alert-rules). Only scalar fields are
// exposed. Reuses sglAlertRuleResponse.

var (
	_ datasource.DataSource              = (*sglAlertRuleDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*sglAlertRuleDataSource)(nil)
)

// NewSGLAlertRuleDataSource is the data source constructor wired into the provider.
func NewSGLAlertRuleDataSource() datasource.DataSource { return &sglAlertRuleDataSource{} }

type sglAlertRuleDataSource struct {
	client *client.Client
}

type sglAlertRuleDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	IsEnabled   types.Bool   `tfsdk:"is_enabled"`
	Severity    types.String `tfsdk:"severity"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (d *sglAlertRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sgl_alert_rule"
}

func (d *sglAlertRuleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *sglAlertRuleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single SGL alert rule by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the SGL alert rule to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "SGL alert rule ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the rule is enabled.",
				Computed:            true,
			},
			"severity": schema.StringAttribute{
				MarkdownDescription: "Severity raised on a breach.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *sglAlertRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config sglAlertRuleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []sglAlertRuleResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/alerting", "/sgl-alert-rules"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading SGL alert rules", err.Error())
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
		config.IsEnabled = types.BoolValue(p.IsEnabled)
		config.Severity = types.StringValue(p.Severity)
		config.CreatedAt = types.StringValue(p.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"SGL alert rule not found",
		fmt.Sprintf("No SGL alert rule named %q was found in the tenant.", wanted),
	)
}
