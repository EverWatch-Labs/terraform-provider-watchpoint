package provider

import (
	"context"
	"os"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the provider satisfies the framework interface.
var _ provider.Provider = (*watchpointProvider)(nil)

// watchpointProvider is the Terraform provider for Watchpoint.
type watchpointProvider struct {
	version string
}

// New returns a provider factory for the given version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &watchpointProvider{version: version}
	}
}

// providerModel maps the provider configuration block.
type providerModel struct {
	APIKey    types.String `tfsdk:"api_key"`
	BaseURL   types.String `tfsdk:"base_url"`
	TenantID  types.String `tfsdk:"tenant_id"`
	Workspace types.String `tfsdk:"workspace"`
}

func (p *watchpointProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "watchpoint"
	resp.Version = p.version
}

func (p *watchpointProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Watchpoint provider configures the Watchpoint observability platform " +
			"(monitors, alerts, dashboards, SLAs, status pages, RUM and more) as code. " +
			"It authenticates with a tenant-scoped API key.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				MarkdownDescription: "Watchpoint API key (the `X-API-Key` value, e.g. `wp_live_...`). " +
					"May also be set via the `WATCHPOINT_API_KEY` environment variable. The key is " +
					"shown only once when created under Settings → API Keys.",
				Optional:  true,
				Sensitive: true,
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Your organization (tenant) ID. All resources are scoped to this tenant. " +
					"May also be set via the `WATCHPOINT_TENANT_ID` environment variable.",
				Optional: true,
			},
			"base_url": schema.StringAttribute{
				MarkdownDescription: "API base URL including the version segment. Defaults to " +
					"`https://api.everwatch-labs.io/api/v1`. May also be set via the " +
					"`WATCHPOINT_BASE_URL` environment variable.",
				Optional: true,
			},
			"workspace": schema.StringAttribute{
				MarkdownDescription: "Workspace name recorded on every monitor this provider creates or updates, " +
					"shown in Watchpoint as \"Managed by Terraform (workspace X)\". Defaults to the " +
					"`TF_WORKSPACE` environment variable, else `default`.",
				Optional: true,
			},
		},
	}
}

func (p *watchpointProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := stringOrEnv(cfg.APIKey, "WATCHPOINT_API_KEY")
	tenantID := stringOrEnv(cfg.TenantID, "WATCHPOINT_TENANT_ID")
	baseURL := stringOrEnv(cfg.BaseURL, "WATCHPOINT_BASE_URL")
	workspace := stringOrEnv(cfg.Workspace, "TF_WORKSPACE")

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			pathRoot("api_key"),
			"Missing Watchpoint API key",
			"Set the `api_key` provider argument or the WATCHPOINT_API_KEY environment variable.",
		)
	}
	if tenantID == "" {
		resp.Diagnostics.AddAttributeError(
			pathRoot("tenant_id"),
			"Missing Watchpoint tenant ID",
			"Set the `tenant_id` provider argument or the WATCHPOINT_TENANT_ID environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c := client.New(client.Config{
		BaseURL:   baseURL,
		APIKey:    apiKey,
		TenantID:  tenantID,
		Workspace: workspace,
		UserAgent: "terraform-provider-watchpoint/" + p.version,
	})
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *watchpointProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// Monitors
		NewHTTPMonitorResource,
		NewSSLMonitorResource,
		NewSyntheticMonitorResource,
		NewDNSMonitorResource,
		NewTCPMonitorResource,
		NewPingMonitorResource,
		NewHeartbeatMonitorResource,
		NewMailMonitorResource,
		NewUDPMonitorResource,
		NewDomainMonitorResource,
		NewMonitorPolicyResource,
		NewTenantMonitoringPolicyResource,
		NewEnvironmentResource,
		// Profiles & alerting
		NewAlertProfileResource,
		NewThresholdProfileResource,
		NewLocationProfileResource,
		NewSLAProfileResource,
		NewCompositeAlertRuleResource,
		NewSGLAlertRuleResource,
		NewOnCallScheduleResource,
		NewEscalationPolicyResource,
		NewAlertRoutingRuleResource,
		NewIncidentSeverityPolicyResource,
		NewWorkflowResource,
		// Organization
		NewMonitorGroupResource,
		NewMonitorGroupMemberResource,
		NewMonitorDependencyResource,
		NewMaintenanceWindowResource,
		NewCalendarFeedResource,
		NewChangeSourceResource,
		NewDeploymentResource,
		NewVaultSecretResource,
		NewExternalVaultResource,
		NewAuthProfileResource,
		// Dashboards & reports
		NewDashboardResource,
		NewDashboardFolderResource,
		NewScheduledReportResource,
		NewReportShareLinkResource,
		// Identity
		NewRoleResource,
		NewAPIKeyResource,
		NewUserGroupResource,
		NewDomainVerificationResource,
		NewSSOConfigurationResource,
		NewSSOGroupMappingResource,
		NewAuditWebhookResource,
		NewInvitationResource,
		NewResourceAclResource,
		NewEventSubscriptionResource,
		NewTenantSecuritySettingsResource,
		NewServiceAccountResource,
		// Agents
		NewAgentEnrollmentTokenResource,
		// Billing
		NewUsageBudgetResource,
		NewUsageAnomalyAlertResource,
		NewCostAllocationTagResource,
		NewCostAllocationSettingsResource,
		// Status pages
		NewStatusPageResource,
		NewStatusPageComponentResource,
		NewStatusPageMaintenanceResource,
		NewStatusPageExternalSourceResource,
		NewStatusPageComponentGroupResource,
		NewStatusPageSubscriberResource,
		NewStatusPageTranslationResource,
		NewIncidentUpdateTemplateResource,
		// RUM
		NewRUMSiteResource,
		// Observability
		NewDerivedMetricRuleResource,
		NewLogPipelineResource,
		NewLogArchiveDestinationResource,
		NewCloudIntegrationResource,
		NewExportSinkResource,
	}
}

func (p *watchpointProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewRegionsDataSource,
		NewMonitorsDataSource,
		NewEnvironmentDataSource,
		NewLocationProfileDataSource,
		NewAlertProfileDataSource,
		NewThresholdProfileDataSource,
		NewMonitorGroupDataSource,
		// Lookup-by-name data sources
		NewSLAProfileDataSource,
		NewEscalationPolicyDataSource,
		NewOnCallScheduleDataSource,
		NewCompositeAlertRuleDataSource,
		NewSGLAlertRuleDataSource,
		NewAlertRoutingRuleDataSource,
		NewDashboardDataSource,
		NewScheduledReportDataSource,
		NewStatusPageDataSource,
		NewUsageBudgetDataSource,
		NewRoleDataSource,
		NewUserGroupDataSource,
		NewSSOConfigurationDataSource,
		NewDomainVerificationDataSource,
		NewMaintenanceWindowDataSource,
		NewRUMSiteDataSource,
		NewAuthProfileDataSource,
		NewVaultSecretDataSource,
		NewExternalVaultFederationDataSource,
		NewAuditWebhookDataSource,
		// Observability
		NewOTLPHostsDataSource,
		NewAnnotationsDataSource,
		NewKubernetesClusterDataSource,
	}
}

// stringOrEnv returns the configured value, falling back to the env var.
func stringOrEnv(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}
