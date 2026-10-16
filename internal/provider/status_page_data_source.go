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

// status_page data source looks up a single status page by name from the tenant
// list (GET /status/.../pages). Reuses statusPageResponse. is_published mirrors
// the resource's is_enabled field.

var (
	_ datasource.DataSource              = (*statusPageDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*statusPageDataSource)(nil)
)

// NewStatusPageDataSource is the data source constructor wired into the provider.
func NewStatusPageDataSource() datasource.DataSource { return &statusPageDataSource{} }

type statusPageDataSource struct {
	client *client.Client
}

type statusPageDataSourceModel struct {
	Name        types.String `tfsdk:"name"`
	ID          types.String `tfsdk:"id"`
	Slug        types.String `tfsdk:"slug"`
	Visibility  types.String `tfsdk:"visibility"`
	IsPublished types.Bool   `tfsdk:"is_published"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (d *statusPageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func (d *statusPageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *statusPageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single status page by name within the tenant.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the status page to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Status page ID.",
				Computed:            true,
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: "Per-tenant unique slug.",
				Computed:            true,
			},
			"visibility": schema.StringAttribute{
				MarkdownDescription: "Page visibility (e.g. `Public`, `Private`).",
				Computed:            true,
			},
			"is_published": schema.BoolAttribute{
				MarkdownDescription: "Whether the page is published.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *statusPageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config statusPageDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []statusPageResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/status", "/pages"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading status pages", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.Slug = types.StringValue(p.Slug)
		config.Visibility = types.StringValue(p.Visibility)
		config.IsPublished = types.BoolValue(p.IsEnabled)
		config.CreatedAt = types.StringValue(p.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Status page not found",
		fmt.Sprintf("No status page named %q was found in the tenant.", wanted),
	)
}
