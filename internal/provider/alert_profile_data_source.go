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

// alert_profile data source looks up a single alert profile by name from the
// tenant list (GET /monitoring/.../alert-profiles). Only the scalar profile
// fields are exposed; the notification channel list is intentionally omitted.

var (
	_ datasource.DataSource              = (*alertProfileDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*alertProfileDataSource)(nil)
)

// NewAlertProfileDataSource is the data source constructor wired into the provider.
func NewAlertProfileDataSource() datasource.DataSource { return &alertProfileDataSource{} }

type alertProfileDataSource struct {
	client *client.Client
}

// alertProfileDataSourceModel maps the resolved profile into state.
type alertProfileDataSourceModel struct {
	Name            types.String `tfsdk:"name"`
	ID              types.String `tfsdk:"id"`
	Description     types.String `tfsdk:"description"`
	IsDefault       types.Bool   `tfsdk:"is_default"`
	IsEnabled       types.Bool   `tfsdk:"is_enabled"`
	IsGlobal        types.Bool   `tfsdk:"is_global"`
	CooldownMinutes types.Int64  `tfsdk:"cooldown_minutes"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

func (d *alertProfileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_profile"
}

func (d *alertProfileDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *alertProfileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single alert profile by name within the tenant. " +
			"Returns the scalar profile fields only; notification channels are not included.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the alert profile to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Alert profile ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Computed:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the tenant's default alert profile.",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the profile is enabled.",
				Computed:            true,
			},
			"is_global": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a global, system-managed profile.",
				Computed:            true,
			},
			"cooldown_minutes": schema.Int64Attribute{
				MarkdownDescription: "Minutes to suppress repeat alerts after firing.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *alertProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config alertProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []alertProfileResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/alerting", "/alert-profiles"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading alert profiles", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		ap := &list[i]
		if !strings.EqualFold(ap.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(ap.ID)
		config.Description = strFromPtr(ap.Description)
		config.IsDefault = types.BoolValue(ap.IsDefault)
		config.IsEnabled = types.BoolValue(ap.IsEnabled)
		config.IsGlobal = types.BoolValue(ap.IsGlobal)
		config.CooldownMinutes = types.Int64Value(ap.CooldownMinutes)
		config.CreatedAt = types.StringValue(ap.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Alert profile not found",
		fmt.Sprintf("No alert profile named %q was found in the tenant.", wanted),
	)
}
