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

// vault_secret data source looks up a single vault secret by key from the tenant
// list (GET /monitoring/.../vault). Reuses vaultSecretResponse.
//
// SECURITY: this data source exposes metadata ONLY. The secret value is never
// surfaced; the list response itself omits it (only `hasValue` is returned).

var (
	_ datasource.DataSource              = (*vaultSecretDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*vaultSecretDataSource)(nil)
)

// NewVaultSecretDataSource is the data source constructor wired into the provider.
func NewVaultSecretDataSource() datasource.DataSource { return &vaultSecretDataSource{} }

type vaultSecretDataSource struct {
	client *client.Client
}

type vaultSecretDataSourceModel struct {
	Key         types.String `tfsdk:"key"`
	ID          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	Category    types.String `tfsdk:"category"`
}

func (d *vaultSecretDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_secret"
}

func (d *vaultSecretDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *vaultSecretDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single vault secret by key within the tenant. " +
			"Exposes metadata only; the secret value is never returned.",
		Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				MarkdownDescription: "Key of the vault secret to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Vault secret ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Computed:            true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: "Grouping category: `credential`, `token`, `certificate`, or `other`.",
				Computed:            true,
			},
		},
	}
}

func (d *vaultSecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config vaultSecretDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []vaultSecretResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/vault"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading vault secrets", err.Error())
		return
	}

	wanted := config.Key.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.Key, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.Description = strFromPtr(p.Description)
		config.Category = strFromPtr(p.Category)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Vault secret not found",
		fmt.Sprintf("No vault secret with key %q was found in the tenant.", wanted),
	)
}
