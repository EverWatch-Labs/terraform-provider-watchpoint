package provider

import (
	"context"
	"fmt"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// kubernetes_cluster looks up the cluster monitor the platform created when a
// Kubernetes operator enrolled (Monitoring, /monitors/kubernetes). The monitor is
// created and removed with the operator, never by a client, so it is a data
// source: its id feeds watchpoint_monitor_policy, SLA attachment, routing and
// the like. Live health (ready nodes, unhealthy pods) is deliberately omitted,
// because a data source is re-read on every plan and a moving number would
// churn anything built from it.

var (
	_ datasource.DataSource              = (*kubernetesClusterDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*kubernetesClusterDataSource)(nil)
)

// NewKubernetesClusterDataSource is the data source constructor wired into the provider.
func NewKubernetesClusterDataSource() datasource.DataSource { return &kubernetesClusterDataSource{} }

type kubernetesClusterDataSource struct {
	client *client.Client
}

type kubernetesClusterModel struct {
	ClusterName    types.String `tfsdk:"cluster_name"`
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	AgentID        types.String `tfsdk:"agent_id"`
	Status         types.String `tfsdk:"status"`
	AlertProfileID types.String `tfsdk:"alert_profile_id"`
	SlaProfileID   types.String `tfsdk:"sla_profile_id"`
	GroupID        types.String `tfsdk:"group_id"`
	Tags           types.Set    `tfsdk:"tags"`
}

type kubernetesClusterRow struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ClusterName    string  `json:"clusterName"`
	AgentID        string  `json:"agentId"`
	Status         string  `json:"status"`
	AlertProfileID *string `json:"alertProfileId"`
	SlaProfileID   *string `json:"slaProfileId"`
	GroupID        *string `json:"groupId"`
	Tags           *string `json:"tags"`
}

func (d *kubernetesClusterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kubernetes_cluster"
}

func (d *kubernetesClusterDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *kubernetesClusterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up the cluster monitor created when a Watchpoint Kubernetes operator " +
			"enrolled, by the `clusterName` the operator was installed with. Use its `id` wherever a monitor " +
			"id is taken, for example `watchpoint_monitor_policy`. Requires the `kubernetes_operator` feature.",
		Attributes: map[string]schema.Attribute{
			"cluster_name": schema.StringAttribute{
				MarkdownDescription: "Cluster name the operator was installed with (Helm `clusterName`).",
				Required:            true,
			},
			"id":               computed("Monitor ID."),
			"name":             computed("Monitor name, `Kubernetes: <cluster_name>` unless renamed."),
			"agent_id":         computed("ID of the enrolled operator that reports for this cluster."),
			"status":           computed("`Active`, `Paused`, or `Disabled` once the operator is removed."),
			"alert_profile_id": computed("Alert profile attached to the monitor."),
			"sla_profile_id":   computed("SLA profile attached to the monitor."),
			"group_id":         computed("Monitor group the monitor belongs to."),
			"tags": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Tags on the monitor. Unordered.",
			},
		},
	}
}

func (d *kubernetesClusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config kubernetesClusterModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var rows []kubernetesClusterRow
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/monitors/kubernetes"), nil, &rows); err != nil {
		resp.Diagnostics.AddError("Error reading Kubernetes cluster monitors", err.Error())
		return
	}

	// A re-installed operator leaves the old monitor Disabled next to the live
	// one under the same cluster name, so prefer a row that is not Disabled.
	var match *kubernetesClusterRow
	for i := range rows {
		if rows[i].ClusterName != config.ClusterName.ValueString() {
			continue
		}
		if match == nil || (match.Status == "Disabled" && rows[i].Status != "Disabled") {
			match = &rows[i]
		}
	}
	if match == nil {
		resp.Diagnostics.AddError("Kubernetes cluster not found",
			fmt.Sprintf("No cluster monitor named %q. Install the operator with that clusterName first.", config.ClusterName.ValueString()))
		return
	}

	config.ID = types.StringValue(match.ID)
	config.Name = types.StringValue(match.Name)
	config.AgentID = strNullIfEmpty(match.AgentID)
	config.Status = types.StringValue(match.Status)
	config.AlertProfileID = strFromPtr(match.AlertProfileID)
	config.SlaProfileID = strFromPtr(match.SlaProfileID)
	config.GroupID = strFromPtr(match.GroupID)
	config.Tags = setFromCSVPtr(ctx, match.Tags, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
