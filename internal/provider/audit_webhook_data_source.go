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

// audit_webhook data source looks up a single audit-log webhook destination by
// name from the tenant list (GET /identity/.../audit-webhooks). Reuses
// auditWebhookListItem. is_enabled mirrors the list item's isActive field.
//
// SECURITY: the HMAC signing secret is never exposed. It is returned by the API
// only once at creation and is absent from the list response this data source
// reads.

var (
	_ datasource.DataSource              = (*auditWebhookDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*auditWebhookDataSource)(nil)
)

// NewAuditWebhookDataSource is the data source constructor wired into the provider.
func NewAuditWebhookDataSource() datasource.DataSource { return &auditWebhookDataSource{} }

type auditWebhookDataSource struct {
	client *client.Client
}

type auditWebhookDataSourceModel struct {
	Name      types.String `tfsdk:"name"`
	ID        types.String `tfsdk:"id"`
	URL       types.String `tfsdk:"url"`
	IsEnabled types.Bool   `tfsdk:"is_enabled"`
}

func (d *auditWebhookDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_audit_webhook"
}

func (d *auditWebhookDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *auditWebhookDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single audit-log webhook destination by name within the tenant. " +
			"The HMAC signing secret is never exposed.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name of the webhook destination to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Webhook destination ID.",
				Computed:            true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "HTTPS URL audit-log events are POSTed to.",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether deliveries are active.",
				Computed:            true,
			},
		},
	}
}

func (d *auditWebhookDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config auditWebhookDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []auditWebhookListItem
	if err := d.client.Get(ctx, d.client.Tenantf("/identity", "/audit-webhooks"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading audit webhooks", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		p := &list[i]
		if !strings.EqualFold(p.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(p.ID)
		config.URL = types.StringValue(p.URL)
		config.IsEnabled = types.BoolValue(p.IsActive)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Audit webhook not found",
		fmt.Sprintf("No audit webhook named %q was found in the tenant.", wanted),
	)
}
