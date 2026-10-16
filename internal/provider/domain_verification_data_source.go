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

// domain_verification data source looks up a single domain verification by domain
// from the tenant list (GET /identity/.../domains). Reuses
// domainVerificationResponse. verified is derived from the verification status.

var (
	_ datasource.DataSource              = (*domainVerificationDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*domainVerificationDataSource)(nil)
)

// NewDomainVerificationDataSource is the data source constructor wired into the provider.
func NewDomainVerificationDataSource() datasource.DataSource { return &domainVerificationDataSource{} }

type domainVerificationDataSource struct {
	client *client.Client
}

type domainVerificationDataSourceModel struct {
	Domain    types.String `tfsdk:"domain"`
	ID        types.String `tfsdk:"id"`
	Verified  types.Bool   `tfsdk:"verified"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (d *domainVerificationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain_verification"
}

func (d *domainVerificationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *domainVerificationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single DNS domain verification by domain within the tenant.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: "Domain to look up (case-insensitive), e.g. `example.com`.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Domain verification ID.",
				Computed:            true,
			},
			"verified": schema.BoolAttribute{
				MarkdownDescription: "Whether the domain has been verified.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *domainVerificationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config domainVerificationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []domainVerificationResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/identity", "/domains"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading domain verifications", err.Error())
		return
	}

	wanted := config.Domain.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.Domain, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.Verified = types.BoolValue(strings.EqualFold(p.Status, "Verified"))
		if p.CreatedAt != nil {
			config.CreatedAt = types.StringValue(*p.CreatedAt)
		} else {
			config.CreatedAt = types.StringNull()
		}

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Domain verification not found",
		fmt.Sprintf("No domain verification for %q was found in the tenant.", wanted),
	)
}
