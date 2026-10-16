package provider

import (
	"context"
	"fmt"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A cloud metric integration (Ingestion, /observability/otlp/cloud-integrations/
// {tenant}): AWS CloudWatch, Azure Monitor or GCP Cloud Monitoring metrics pulled
// every five minutes into the tenant's metrics. Credentials are write-only (the
// AWS key id comes back masked), so the configured values are kept in state and
// re-sent on every update.

var (
	_ resource.Resource                = (*cloudIntegrationResource)(nil)
	_ resource.ResourceWithConfigure   = (*cloudIntegrationResource)(nil)
	_ resource.ResourceWithImportState = (*cloudIntegrationResource)(nil)
)

// NewCloudIntegrationResource is the resource constructor wired into the provider.
func NewCloudIntegrationResource() resource.Resource { return &cloudIntegrationResource{} }

type cloudIntegrationResource struct {
	client *client.Client
}

type cloudIntegrationModel struct {
	ID                 types.String `tfsdk:"id"`
	Provider           types.String `tfsdk:"provider_type"`
	Name               types.String `tfsdk:"name"`
	ResourceTypes      types.Set    `tfsdk:"resource_types"`
	Regions            types.Set    `tfsdk:"regions"`
	AccessKeyID        types.String `tfsdk:"access_key_id"`
	SecretAccessKey    types.String `tfsdk:"secret_access_key"`
	RoleArn            types.String `tfsdk:"role_arn"`
	DirectoryID        types.String `tfsdk:"directory_id"`
	ClientID           types.String `tfsdk:"client_id"`
	ClientSecret       types.String `tfsdk:"client_secret"`
	SubscriptionID     types.String `tfsdk:"subscription_id"`
	ServiceAccountJSON types.String `tfsdk:"service_account_json"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	HasSecret          types.Bool   `tfsdk:"has_secret"`
	ProjectID          types.String `tfsdk:"project_id"`
	ClientEmail        types.String `tfsdk:"client_email"`
}

type cloudIntegrationBody struct {
	Provider           string   `json:"provider"`
	Name               *string  `json:"name,omitempty"`
	ResourceTypes      []string `json:"resourceTypes"`
	Regions            []string `json:"regions,omitempty"`
	AccessKeyID        *string  `json:"accessKeyId,omitempty"`
	SecretAccessKey    *string  `json:"secretAccessKey,omitempty"`
	RoleArn            *string  `json:"roleArn,omitempty"`
	DirectoryID        *string  `json:"directoryId,omitempty"`
	ClientID           *string  `json:"clientId,omitempty"`
	ClientSecret       *string  `json:"clientSecret,omitempty"`
	SubscriptionID     *string  `json:"subscriptionId,omitempty"`
	ServiceAccountJSON *string  `json:"serviceAccountJson,omitempty"`
	Enabled            *bool    `json:"enabled,omitempty"`
}

type cloudIntegrationResponse struct {
	ID             string   `json:"id"`
	Provider       string   `json:"provider"`
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	ResourceTypes  []string `json:"resourceTypes"`
	Regions        []string `json:"regions"`
	RoleArn        *string  `json:"roleArn"`
	DirectoryID    *string  `json:"directoryId"`
	ClientID       *string  `json:"clientId"`
	SubscriptionID *string  `json:"subscriptionId"`
	ProjectID      *string  `json:"projectId"`
	ClientEmail    *string  `json:"clientEmail"`
	HasSecret      bool     `json:"hasSecret"`
}

func (r *cloudIntegrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cloud_integration"
}

func (r *cloudIntegrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *cloudIntegrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A cloud metric integration: AWS CloudWatch, Azure Monitor or GCP Cloud Monitoring " +
			"metrics for the chosen resource types are pulled every five minutes and stored as OTLP gauges " +
			"(billed as metric datapoints). At most 10 per tenant. Requires the `cloud_integrations` feature " +
			"(Pro). Credentials are write-only: they are kept in state from configuration and a change made " +
			"in the portal is not detected.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Integration ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"provider_type": schema.StringAttribute{
				MarkdownDescription: "`aws`, `azure` or `gcp`. Changing it replaces the integration.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf("aws", "azure", "gcp")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name (100 characters max). Defaults to the provider in capitals.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"resource_types": schema.SetAttribute{
				MarkdownDescription: "Resource types to collect, keys from the integration catalog, for example " +
					"`ec2`, `rds`, `lambda` for AWS. Unknown keys are rejected.",
				Required:    true,
				ElementType: types.StringType,
			},
			"regions": schema.SetAttribute{
				MarkdownDescription: "`aws`: regions to read. Defaults to `us-east-1`.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"access_key_id": schema.StringAttribute{
				MarkdownDescription: "`aws`: access key id (`AKIA...`). Use either a key or `role_arn`.",
				Optional:            true,
			},
			"secret_access_key": schema.StringAttribute{
				MarkdownDescription: "`aws`: secret access key.",
				Optional:            true,
				Sensitive:           true,
			},
			"role_arn": schema.StringAttribute{
				MarkdownDescription: "`aws`: IAM role to assume, trusted for the external id " +
					"`watchpoint-<tenant id>`. Only where the platform offers role access.",
				Optional: true,
			},
			"directory_id": schema.StringAttribute{
				MarkdownDescription: "`azure`: directory (tenant) id.",
				Optional:            true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "`azure`: application (client) id holding Monitoring Reader.",
				Optional:            true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "`azure`: client secret.",
				Optional:            true,
				Sensitive:           true,
			},
			"subscription_id": schema.StringAttribute{
				MarkdownDescription: "`azure`: subscription to read.",
				Optional:            true,
			},
			"service_account_json": schema.StringAttribute{
				MarkdownDescription: "`gcp`: service account JSON key with Monitoring Viewer.",
				Optional:            true,
				Sensitive:           true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the integration is polled. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"has_secret": schema.BoolAttribute{
				MarkdownDescription: "Whether a secret credential is stored.",
				Computed:            true,
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "`gcp`: project read from the service account key.",
				Computed:            true,
			},
			"client_email": schema.StringAttribute{
				MarkdownDescription: "`gcp`: service account email read from the key.",
				Computed:            true,
			},
		},
	}
}

func (r *cloudIntegrationResource) path(suffix string) string {
	return fmt.Sprintf("/observability/otlp/cloud-integrations/%s%s", r.client.TenantID(), suffix)
}

func (m *cloudIntegrationModel) toBody(ctx context.Context, ds *diagSink) *cloudIntegrationBody {
	return &cloudIntegrationBody{
		Provider:           m.Provider.ValueString(),
		Name:               strPtr(m.Name),
		ResourceTypes:      emptyIfNil(stringSliceFromSet(ctx, m.ResourceTypes, ds.d)),
		Regions:            stringSliceFromSet(ctx, m.Regions, ds.d),
		AccessKeyID:        strPtr(m.AccessKeyID),
		SecretAccessKey:    strPtr(m.SecretAccessKey),
		RoleArn:            strPtr(m.RoleArn),
		DirectoryID:        strPtr(m.DirectoryID),
		ClientID:           strPtr(m.ClientID),
		ClientSecret:       strPtr(m.ClientSecret),
		SubscriptionID:     strPtr(m.SubscriptionID),
		ServiceAccountJSON: strPtr(m.ServiceAccountJSON),
		Enabled:            boolPtr(m.Enabled),
	}
}

// applyResponse leaves the credential inputs as configured: none are returned
// in clear (the AWS key id is masked).
func (m *cloudIntegrationModel) applyResponse(ctx context.Context, in *cloudIntegrationResponse, ds *diagSink) {
	m.ID = types.StringValue(in.ID)
	m.Provider = types.StringValue(in.Provider)
	m.Name = types.StringValue(in.Name)
	m.Enabled = types.BoolValue(in.Enabled)
	m.ResourceTypes = setFromStringSlice(ctx, emptyIfNil(in.ResourceTypes), ds.d)
	m.Regions = setOrNull(ctx, in.Regions, ds)
	m.RoleArn = strFromPtr(in.RoleArn)
	m.DirectoryID = strFromPtr(in.DirectoryID)
	m.ClientID = strFromPtr(in.ClientID)
	m.SubscriptionID = strFromPtr(in.SubscriptionID)
	m.ProjectID = strFromPtr(in.ProjectID)
	m.ClientEmail = strFromPtr(in.ClientEmail)
	m.HasSecret = types.BoolValue(in.HasSecret)
}

func (r *cloudIntegrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cloudIntegrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	var got cloudIntegrationResponse
	if err := r.client.Post(ctx, r.path(""), plan.toBody(ctx, ds), &got); err != nil {
		resp.Diagnostics.AddError("Error creating cloud integration", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *cloudIntegrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cloudIntegrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var list struct {
		Integrations []cloudIntegrationResponse `json:"integrations"`
	}
	if err := r.client.Get(ctx, r.path(""), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading cloud integration", err.Error())
		return
	}
	for i := range list.Integrations {
		if list.Integrations[i].ID == state.ID.ValueString() {
			state.applyResponse(ctx, &list.Integrations[i], &diagSink{d: &resp.Diagnostics})
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *cloudIntegrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan cloudIntegrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	var got cloudIntegrationResponse
	if err := r.client.Put(ctx, r.path("/"+plan.ID.ValueString()), plan.toBody(ctx, ds), &got); err != nil {
		resp.Diagnostics.AddError("Error updating cloud integration", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *cloudIntegrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cloudIntegrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path("/"+state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting cloud integration", err.Error())
	}
}

func (r *cloudIntegrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
