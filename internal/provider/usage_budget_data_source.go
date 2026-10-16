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

// usage_budget data source looks up a single usage budget by name from the
// tenant list (GET /billing/.../budgets). Reuses usageBudgetResponse.

var (
	_ datasource.DataSource              = (*usageBudgetDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*usageBudgetDataSource)(nil)
)

// NewUsageBudgetDataSource is the data source constructor wired into the provider.
func NewUsageBudgetDataSource() datasource.DataSource { return &usageBudgetDataSource{} }

type usageBudgetDataSource struct {
	client *client.Client
}

type usageBudgetDataSourceModel struct {
	Name           types.String `tfsdk:"name"`
	ID             types.String `tfsdk:"id"`
	ThresholdCents types.Int64  `tfsdk:"threshold_cents"`
	ProductSlug    types.String `tfsdk:"product_slug"`
	Mode           types.String `tfsdk:"mode"`
	NotifyEmail    types.String `tfsdk:"notify_email"`
	IsEnabled      types.Bool   `tfsdk:"is_enabled"`
}

func (d *usageBudgetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_usage_budget"
}

func (d *usageBudgetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *usageBudgetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single usage budget by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the usage budget to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Usage budget ID.",
				Computed:            true,
			},
			"threshold_cents": schema.Int64Attribute{
				MarkdownDescription: "Spend threshold in cents.",
				Computed:            true,
			},
			"product_slug": schema.StringAttribute{
				MarkdownDescription: "Product slug the budget is scoped to, if any.",
				Computed:            true,
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: "Budget evaluation mode: `forecast` or `actual`.",
				Computed:            true,
			},
			"notify_email": schema.StringAttribute{
				MarkdownDescription: "Email address notified when the budget is breached.",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the budget is active.",
				Computed:            true,
			},
		},
	}
}

func (d *usageBudgetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config usageBudgetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []usageBudgetResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/billing", "/budgets"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading usage budgets", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.ThresholdCents = types.Int64Value(p.ThresholdCents)
		config.ProductSlug = strFromPtr(p.ProductSlug)
		config.Mode = types.StringValue(p.Mode)
		config.NotifyEmail = strFromPtr(p.NotifyEmail)
		config.IsEnabled = types.BoolValue(p.IsEnabled)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Usage budget not found",
		fmt.Sprintf("No usage budget named %q was found in the tenant.", wanted),
	)
}
