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

// composite_alert_rule data source looks up a single composite alert rule by name
// from the tenant list (GET /alerting/.../alert-rules). Only scalar fields are
// exposed; conditions are omitted. Reuses compositeAlertRuleResponse.

var (
	_ datasource.DataSource              = (*compositeAlertRuleDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*compositeAlertRuleDataSource)(nil)
)

// NewCompositeAlertRuleDataSource is the data source constructor wired into the provider.
func NewCompositeAlertRuleDataSource() datasource.DataSource { return &compositeAlertRuleDataSource{} }

type compositeAlertRuleDataSource struct {
	client *client.Client
}

type compositeAlertRuleDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	IsEnabled   types.Bool   `tfsdk:"is_enabled"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (d *compositeAlertRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_composite_alert_rule"
}

func (d *compositeAlertRuleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *compositeAlertRuleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// Deprecated alongside the resource: same retired API, same replacement.
		DeprecationMessage: compositeAlertRuleDeprecation,
		MarkdownDescription: "**Deprecated.** Looks up a single composite alert rule by name within the tenant. " +
			"Returns the scalar rule fields only; conditions are not included. Composite rules are no longer " +
			"evaluated by the platform and this data source will be removed in a future release. Express the same " +
			"combination as an SGL alert rule and use `watchpoint_sgl_alert_rule` instead (Alerting, " +
			"`/alerting/rules`).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the composite alert rule to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Composite alert rule ID.",
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
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *compositeAlertRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config compositeAlertRuleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []compositeAlertRuleResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/alerting", "/alert-rules"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading composite alert rules", err.Error())
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
		config.CreatedAt = types.StringValue(p.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Composite alert rule not found",
		fmt.Sprintf("No composite alert rule named %q was found in the tenant.", wanted),
	)
}
