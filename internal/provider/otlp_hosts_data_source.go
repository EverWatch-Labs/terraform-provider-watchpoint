package provider

import (
	"context"
	"fmt"
	"net/url"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// otlp_hosts lists the hosts the tenant's OTLP host metrics have been seen from
// inside a rolling window, so a configuration can drive monitors, tags or
// dashboards off the fleet that actually reports rather than a hand-maintained
// list. Read-only reporting belongs in a data source, never a resource: nothing
// here is tenant configuration and nothing here can be reconciled.
//
// ONLY THE STABLE IDENTITY of a host is exposed. The endpoint also returns ~20
// gauges and counters per host (CPU, memory, disk and network levels for the
// window), which are deliberately left out: a data source is re-read on every
// plan, so surfacing a number that moves every minute would churn anything that
// referenced it. Read those in the portal, where the window is a control rather
// than a plan input.
//
// The window is a ROLLING one (the last 24 hours by default, capped by the API's
// own scan limit), so a host that has stopped reporting leaves the list. A
// for_each over `hosts` therefore destroys the resources built from a host that
// goes quiet - which is the point when the fleet is truly dynamic, and a footgun
// when it is not. `from` / `to` pin the window when a wider one is wanted.
//
// Requires the `apm_hosts` feature flag on the tenant.

var (
	_ datasource.DataSource              = (*otlpHostsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*otlpHostsDataSource)(nil)
)

// NewOTLPHostsDataSource is the data source constructor wired into the provider.
func NewOTLPHostsDataSource() datasource.DataSource { return &otlpHostsDataSource{} }

type otlpHostsDataSource struct {
	client *client.Client
}

// otlpHostModel maps a single host's identity into state.
type otlpHostModel struct {
	HostName types.String `tfsdk:"host_name"`
	HostID   types.String `tfsdk:"host_id"`
	OSType   types.String `tfsdk:"os_type"`
	Arch     types.String `tfsdk:"arch"`
	Services types.Set    `tfsdk:"services"`
	LastSeen types.String `tfsdk:"last_seen"`
}

type otlpHostsDataSourceModel struct {
	From  types.String    `tfsdk:"from"`
	To    types.String    `tfsdk:"to"`
	Hosts []otlpHostModel `tfsdk:"hosts"`
}

// otlpHostRow mirrors the identity half of HostInventoryRow; the per-window
// metric fields are intentionally not decoded.
type otlpHostRow struct {
	HostName string   `json:"hostName"`
	HostID   *string  `json:"hostId"`
	OSType   *string  `json:"osType"`
	Arch     *string  `json:"arch"`
	Services []string `json:"services"`
	LastSeen string   `json:"lastSeen"`
}

// otlpHostsResponse mirrors HostInventoryResponse.
type otlpHostsResponse struct {
	Hosts []otlpHostRow `json:"hosts"`
}

func (d *otlpHostsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_otlp_hosts"
}

func (d *otlpHostsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *otlpHostsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the hosts the tenant's OTLP host metrics have been observed from in a " +
			"rolling window, with the services seen on each. Only stable identity is returned; the live " +
			"CPU, memory, disk and network levels the same endpoint carries are deliberately omitted, " +
			"because a data source is re-read on every plan and a moving number would churn anything " +
			"built from it. Requires the `apm_hosts` feature flag.",
		Attributes: map[string]schema.Attribute{
			"from": schema.StringAttribute{
				MarkdownDescription: "Optional window start (RFC 3339, UTC). Defaults to 24 hours before `to`. " +
					"The API clamps the window to its own maximum scan length.",
				Optional: true,
			},
			"to": schema.StringAttribute{
				MarkdownDescription: "Optional window end (RFC 3339, UTC). Defaults to now.",
				Optional:            true,
			},
			"hosts": schema.ListNestedAttribute{
				MarkdownDescription: "Hosts seen in the window, most recently seen first.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"host_name": schema.StringAttribute{Computed: true, MarkdownDescription: "The host's `host.name` resource attribute."},
						"host_id":   schema.StringAttribute{Computed: true, MarkdownDescription: "The host's `host.id` resource attribute, if it reports one."},
						"os_type":   schema.StringAttribute{Computed: true, MarkdownDescription: "Reported OS type (`linux`, `windows`, ...), if known."},
						"arch":      schema.StringAttribute{Computed: true, MarkdownDescription: "Reported CPU architecture (`amd64`, `arm64`, ...), if known."},
						"services": schema.SetAttribute{
							MarkdownDescription: "Service names observed on this host in the window. A Set: the " +
								"membership is unordered, so comparing it by position would fail.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"last_seen": schema.StringAttribute{Computed: true, MarkdownDescription: "Most recent hour the host reported (RFC 3339)."},
					},
				},
			},
		},
	}
}

func (d *otlpHostsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config otlpHostsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query := url.Values{}
	if !config.From.IsNull() {
		query.Set("fromIso", config.From.ValueString())
	}
	if !config.To.IsNull() {
		query.Set("toIso", config.To.ValueString())
	}

	// The host routes carry the tenant id at the END of the prefix rather than in
	// the "/tenants/<id>/" position every other service uses, so client.Tenantf
	// cannot build them.
	path := fmt.Sprintf("/observability/otlp/hosts/%s", d.client.TenantID())

	var got otlpHostsResponse
	if err := d.client.Get(ctx, path, query, &got); err != nil {
		resp.Diagnostics.AddError("Error reading OTLP hosts", err.Error())
		return
	}

	config.Hosts = make([]otlpHostModel, 0, len(got.Hosts))
	for i := range got.Hosts {
		h := &got.Hosts[i]
		config.Hosts = append(config.Hosts, otlpHostModel{
			HostName: types.StringValue(h.HostName),
			HostID:   strFromPtr(h.HostID),
			OSType:   strFromPtr(h.OSType),
			Arch:     strFromPtr(h.Arch),
			Services: setFromStringSlice(ctx, h.Services, &resp.Diagnostics),
			LastSeen: types.StringValue(h.LastSeen),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
