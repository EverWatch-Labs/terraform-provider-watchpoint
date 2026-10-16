package provider

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// monitors data source lists a tenant's monitors (all types) as summaries,
// with optional type/status filters.

var (
	_ datasource.DataSource              = (*monitorsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*monitorsDataSource)(nil)
)

// NewMonitorsDataSource is the data source constructor wired into the provider.
func NewMonitorsDataSource() datasource.DataSource { return &monitorsDataSource{} }

type monitorsDataSource struct {
	client *client.Client
}

// monitorSummaryModel maps a single monitor summary into state.
type monitorSummaryModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Type                 types.String `tfsdk:"type"`
	Status               types.String `tfsdk:"status"`
	Target               types.String `tfsdk:"target"`
	CheckIntervalSeconds types.Int64  `tfsdk:"check_interval_seconds"`
	LocationProfileName  types.String `tfsdk:"location_profile_name"`
	ThresholdProfileName types.String `tfsdk:"threshold_profile_name"`
	Tags                 types.String `tfsdk:"tags"`
	LastCheckAt          types.String `tfsdk:"last_check_at"`
	LastCheckStatus      types.String `tfsdk:"last_check_status"`
	CreatedAt            types.String `tfsdk:"created_at"`
}

type monitorsDataSourceModel struct {
	Type     types.String          `tfsdk:"type"`
	Status   types.String          `tfsdk:"status"`
	Monitors []monitorSummaryModel `tfsdk:"monitors"`
}

// monitorSummaryResponse mirrors MonitorSummaryResponse.
type monitorSummaryResponse struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Type                 string  `json:"type"`
	Status               string  `json:"status"`
	Target               *string `json:"target"`
	CheckIntervalSeconds int64   `json:"checkIntervalSeconds"`
	LocationProfileName  *string `json:"locationProfileName"`
	ThresholdProfileName *string `json:"thresholdProfileName"`
	Tags                 *string `json:"tags"`
	LastCheckAt          *string `json:"lastCheckAt"`
	LastCheckStatus      *string `json:"lastCheckStatus"`
	CreatedAt            string  `json:"createdAt"`
}

// monitorsPagedResponse mirrors PagedResponse<MonitorSummaryResponse>.
type monitorsPagedResponse struct {
	Items      []monitorSummaryResponse `json:"items"`
	TotalCount int                      `json:"totalCount"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"pageSize"`
	TotalPages int                      `json:"totalPages"`
}

func (d *monitorsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitors"
}

func (d *monitorsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *monitorsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the tenant's monitors (all types) as summaries, optionally filtered by type or status.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				MarkdownDescription: "Optional monitor type filter (e.g. `HttpApi`, `Ssl`, `Synthetic`, `Dns`, `Tcp`, `Ping`).",
				Optional:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Optional monitor status filter (e.g. `Active`, `Paused`, `Maintenance`).",
				Optional:            true,
			},
			"monitors": schema.ListNestedAttribute{
				MarkdownDescription: "Matching monitor summaries.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                     schema.StringAttribute{Computed: true, MarkdownDescription: "Monitor ID."},
						"name":                   schema.StringAttribute{Computed: true, MarkdownDescription: "Monitor name."},
						"type":                   schema.StringAttribute{Computed: true, MarkdownDescription: "Monitor type."},
						"status":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Monitor status."},
						"target":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Monitored target (URL or host:port)."},
						"check_interval_seconds": schema.Int64Attribute{Computed: true, MarkdownDescription: "Seconds between checks."},
						"location_profile_name":  schema.StringAttribute{Computed: true, MarkdownDescription: "Location profile name, if assigned."},
						"threshold_profile_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Threshold profile name, if assigned."},
						"tags":                   schema.StringAttribute{Computed: true, MarkdownDescription: "Comma-separated tags (CSV string)."},
						"last_check_at":          schema.StringAttribute{Computed: true, MarkdownDescription: "Timestamp of the last check (RFC 3339), if any."},
						"last_check_status":      schema.StringAttribute{Computed: true, MarkdownDescription: "Result status of the last check, if any."},
						"created_at":             schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp (RFC 3339)."},
					},
				},
			},
		},
	}
}

func (d *monitorsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config monitorsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query := url.Values{}
	if !config.Type.IsNull() {
		query.Set("type", config.Type.ValueString())
	}
	if !config.Status.IsNull() {
		query.Set("status", config.Status.ValueString())
	}
	// Large pageSize to fetch everything in a single request; the API clamps
	// it server-side if it exceeds the maximum.
	query.Set("pageSize", strconv.Itoa(1000))

	path := d.client.Tenantf("/monitoring", "/monitors")
	var got monitorsPagedResponse
	if err := d.client.Get(ctx, path, query, &got); err != nil {
		resp.Diagnostics.AddError("Error reading monitors", err.Error())
		return
	}

	config.Monitors = make([]monitorSummaryModel, 0, len(got.Items))
	for i := range got.Items {
		m := &got.Items[i]
		config.Monitors = append(config.Monitors, monitorSummaryModel{
			ID:                   types.StringValue(m.ID),
			Name:                 types.StringValue(m.Name),
			Type:                 types.StringValue(m.Type),
			Status:               types.StringValue(m.Status),
			Target:               strFromPtr(m.Target),
			CheckIntervalSeconds: types.Int64Value(m.CheckIntervalSeconds),
			LocationProfileName:  strFromPtr(m.LocationProfileName),
			ThresholdProfileName: strFromPtr(m.ThresholdProfileName),
			Tags:                 strFromPtr(m.Tags),
			LastCheckAt:          strFromPtr(m.LastCheckAt),
			LastCheckStatus:      strFromPtr(m.LastCheckStatus),
			CreatedAt:            types.StringValue(m.CreatedAt),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
