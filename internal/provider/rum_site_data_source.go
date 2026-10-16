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

// rum_site data source looks up a single RUM site by name from the tenant list
// (GET /rum/.../sites). Reuses rumSiteResponse. domain mirrors the resource's
// primary_origin field.

var (
	_ datasource.DataSource              = (*rumSiteDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*rumSiteDataSource)(nil)
)

// NewRUMSiteDataSource is the data source constructor wired into the provider.
func NewRUMSiteDataSource() datasource.DataSource { return &rumSiteDataSource{} }

type rumSiteDataSource struct {
	client *client.Client
}

type rumSiteDataSourceModel struct {
	Name      types.String `tfsdk:"name"`
	ID        types.String `tfsdk:"id"`
	Domain    types.String `tfsdk:"domain"`
	PublicKey types.String `tfsdk:"public_key"`
	IsEnabled types.Bool   `tfsdk:"is_enabled"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (d *rumSiteDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rum_site"
}

func (d *rumSiteDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *rumSiteDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single Real User Monitoring (RUM) site by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the RUM site to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "RUM site ID.",
				Computed:            true,
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "Primary site origin.",
				Computed:            true,
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "Server-generated public key embedded in the install snippet.",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the site is collecting.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *rumSiteDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config rumSiteDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []rumSiteResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/rum", "/sites"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading RUM sites", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.Domain = types.StringValue(p.PrimaryOrigin)
		config.PublicKey = types.StringValue(p.PublicKey)
		config.IsEnabled = types.BoolValue(p.IsEnabled)
		config.CreatedAt = types.StringValue(p.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"RUM site not found",
		fmt.Sprintf("No RUM site named %q was found in the tenant.", wanted),
	)
}
