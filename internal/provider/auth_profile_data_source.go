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

// auth_profile data source looks up a single auth profile by name from the tenant
// list (GET /monitoring/.../auth-profiles). Reuses authProfileResponse.
//
// SECURITY: this data source exposes credential METADATA ONLY. The encrypted
// client secret, password, refresh token, static key and client cert / private
// key (PEM) are masked by the API (it returns only `has*` booleans, never the
// values) and are deliberately NOT surfaced here.

var (
	_ datasource.DataSource              = (*authProfileDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*authProfileDataSource)(nil)
)

// NewAuthProfileDataSource is the data source constructor wired into the provider.
func NewAuthProfileDataSource() datasource.DataSource { return &authProfileDataSource{} }

type authProfileDataSource struct {
	client *client.Client
}

type authProfileDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Kind        types.String `tfsdk:"kind"`
	Description types.String `tfsdk:"description"`
	TokenPrefix types.String `tfsdk:"token_prefix"`
}

func (d *authProfileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_profile"
}

func (d *authProfileDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *authProfileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single auth profile by name within the tenant. " +
			"Exposes credential metadata only; the encrypted client secret, password, refresh token, " +
			"static key and client certificate / private key are never returned.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the auth profile to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Auth profile ID.",
				Computed:            true,
			},
			"kind": schema.StringAttribute{
				MarkdownDescription: "Credential kind (e.g. `OAuth2ClientCredentials`, `StaticApiKey`, `Mtls`).",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Computed:            true,
			},
			"token_prefix": schema.StringAttribute{
				MarkdownDescription: "Prefix prepended to the token when injected into requests.",
				Computed:            true,
			},
		},
	}
}

func (d *authProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config authProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []authProfileResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/auth-profiles"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading auth profiles", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.Kind = types.StringValue(p.Kind)
		config.Description = strFromPtr(p.Description)
		config.TokenPrefix = strFromPtr(p.TokenPrefix)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Auth profile not found",
		fmt.Sprintf("No auth profile named %q was found in the tenant.", wanted),
	)
}
