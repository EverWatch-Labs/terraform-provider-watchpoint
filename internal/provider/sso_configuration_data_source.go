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

// sso_configuration data source looks up a single SSO configuration by its
// connection name from the tenant list (GET /identity/.../sso). Reuses
// ssoConfigurationResponse.
//
// SECURITY: the client secret and SAML certificate / private key are never
// exposed by this data source. The masked list/get response itself never echoes
// those secrets back, and this data source surfaces only non-sensitive metadata
// (id, provider_type, is_enabled).

var (
	_ datasource.DataSource              = (*ssoConfigurationDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*ssoConfigurationDataSource)(nil)
)

// NewSSOConfigurationDataSource is the data source constructor wired into the provider.
func NewSSOConfigurationDataSource() datasource.DataSource { return &ssoConfigurationDataSource{} }

type ssoConfigurationDataSource struct {
	client *client.Client
}

type ssoConfigurationDataSourceModel struct {
	Name         types.String `tfsdk:"name"`
	ID           types.String `tfsdk:"id"`
	ProviderType types.String `tfsdk:"provider_type"`
	IsEnabled    types.Bool   `tfsdk:"is_enabled"`
}

func (d *ssoConfigurationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sso_configuration"
}

func (d *ssoConfigurationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *ssoConfigurationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single SSO configuration by its connection name within the tenant. " +
			"Only non-sensitive metadata is exposed; the client secret and SAML certificate / private key are never returned.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Connection name of the SSO configuration to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "SSO configuration ID.",
				Computed:            true,
			},
			"provider_type": schema.StringAttribute{
				MarkdownDescription: "SSO provider type (e.g. `EntraId`, `Okta`, `OidcGeneric`).",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the connection is enabled.",
				Computed:            true,
			},
		},
	}
}

func (d *ssoConfigurationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ssoConfigurationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []ssoConfigurationResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/identity", "/sso"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading SSO configurations", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.ConnectionName, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.ProviderType = types.StringValue(p.ProviderType)
		config.IsEnabled = types.BoolValue(p.IsEnabled)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"SSO configuration not found",
		fmt.Sprintf("No SSO configuration named %q was found in the tenant.", wanted),
	)
}
