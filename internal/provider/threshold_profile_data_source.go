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

// threshold_profile data source looks up a single threshold profile by name
// from the tenant list (GET /monitoring/.../threshold-profiles). Only the
// scalar profile fields are exposed; the nested rules are intentionally omitted.

var (
	_ datasource.DataSource              = (*thresholdProfileDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*thresholdProfileDataSource)(nil)
)

// NewThresholdProfileDataSource is the data source constructor wired into the provider.
func NewThresholdProfileDataSource() datasource.DataSource { return &thresholdProfileDataSource{} }

type thresholdProfileDataSource struct {
	client *client.Client
}

// thresholdProfileDataSourceModel maps the resolved profile into state.
type thresholdProfileDataSourceModel struct {
	Name      types.String `tfsdk:"name"`
	ID        types.String `tfsdk:"id"`
	IsDefault types.Bool   `tfsdk:"is_default"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (d *thresholdProfileDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_threshold_profile"
}

func (d *thresholdProfileDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *thresholdProfileDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single threshold profile by name within the tenant. " +
			"Returns the scalar profile fields only; threshold rules are not included.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the threshold profile to look up (case-insensitive).",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Threshold profile ID.",
				Computed:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the tenant's default threshold profile.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (d *thresholdProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config thresholdProfileDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list []thresholdProfileResponse
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/threshold-profiles"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading threshold profiles", err.Error())
		return
	}

	wanted := config.Name.ValueString()
	for i := range list {
		tp := &list[i]
		if !strings.EqualFold(tp.Name, wanted) {
			continue
		}

		config.ID = types.StringValue(tp.ID)
		config.IsDefault = types.BoolValue(tp.IsDefault)
		config.CreatedAt = types.StringValue(tp.CreatedAt)

		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	resp.Diagnostics.AddError(
		"Threshold profile not found",
		fmt.Sprintf("No threshold profile named %q was found in the tenant.", wanted),
	)
}
