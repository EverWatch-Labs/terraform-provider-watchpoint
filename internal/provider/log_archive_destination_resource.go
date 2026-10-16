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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The tenant's log archive destination (Ingestion, /observability/otlp/log-archive/
// {tenant}): closed days of logs are written there hourly as gzip JSONL. One per
// tenant. The credential is write-only, so every PUT sends it again from config
// and state keeps the configured value.

var (
	_ resource.Resource                = (*logArchiveDestinationResource)(nil)
	_ resource.ResourceWithConfigure   = (*logArchiveDestinationResource)(nil)
	_ resource.ResourceWithImportState = (*logArchiveDestinationResource)(nil)
)

// NewLogArchiveDestinationResource is the resource constructor wired into the provider.
func NewLogArchiveDestinationResource() resource.Resource { return &logArchiveDestinationResource{} }

type logArchiveDestinationResource struct {
	client *client.Client
}

type logArchiveDestinationModel struct {
	ID              types.String `tfsdk:"id"`
	Type            types.String `tfsdk:"type"`
	SasURL          types.String `tfsdk:"sas_url"`
	Bucket          types.String `tfsdk:"bucket"`
	Region          types.String `tfsdk:"region"`
	Endpoint        types.String `tfsdk:"endpoint"`
	AccessKeyID     types.String `tfsdk:"access_key_id"`
	SecretAccessKey types.String `tfsdk:"secret_access_key"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	BaseURL         types.String `tfsdk:"base_url"`
}

type logArchiveBody struct {
	Type            string  `json:"type"`
	URL             *string `json:"url,omitempty"`
	Bucket          *string `json:"bucket,omitempty"`
	Region          *string `json:"region,omitempty"`
	Endpoint        *string `json:"endpoint,omitempty"`
	AccessKeyID     *string `json:"accessKeyId,omitempty"`
	SecretAccessKey *string `json:"secretAccessKey,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
}

type logArchiveDestinationResponse struct {
	Type    string  `json:"type"`
	BaseURL string  `json:"baseUrl"`
	Region  *string `json:"region"`
	Enabled bool    `json:"enabled"`
}

func (r *logArchiveDestinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_log_archive_destination"
}

func (r *logArchiveDestinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *logArchiveDestinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Where the tenant's logs are archived: each closed UTC day is written hour by hour " +
			"as gzip JSONL under `watchpoint-logs/<tenant>/dt=YYYY-MM-DD/hour=HH/service=NAME/`, readable by " +
			"Athena, Spark or Synapse in place and rehydratable into Watchpoint. A per-tenant singleton. " +
			"Requires the `log_archives` feature (Scale). Credentials are write-only and kept in state from " +
			"configuration; changing the destination starts a new archive from yesterday.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tenant ID, which this setting is keyed on.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "`azure_blob` or `s3` (AWS or any S3-compatible store).",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf("azure_blob", "s3")},
			},
			"sas_url": schema.StringAttribute{
				MarkdownDescription: "`azure_blob`: container SAS URL, `https://<account>.blob.core.windows.net/" +
					"<container>?sv=...&sig=...`. The query string is the credential.",
				Optional:  true,
				Sensitive: true,
			},
			"bucket": schema.StringAttribute{
				MarkdownDescription: "`s3`: bucket name. AWS buckets with dots are not supported.",
				Optional:            true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "`s3`: region, for example `us-east-1` (`auto` for R2). Required for AWS; " +
					"defaults to `us-east-1` with a custom `endpoint`.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "`s3`: https endpoint with no path for an S3-compatible store (MinIO, R2). " +
					"Omit for AWS.",
				Optional: true,
			},
			"access_key_id": schema.StringAttribute{
				MarkdownDescription: "`s3`: access key id.",
				Optional:            true,
			},
			"secret_access_key": schema.StringAttribute{
				MarkdownDescription: "`s3`: secret access key.",
				Optional:            true,
				Sensitive:           true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether archiving runs. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"base_url": schema.StringAttribute{
				MarkdownDescription: "Resolved destination URL, without the credential.",
				Computed:            true,
			},
		},
	}
}

func (r *logArchiveDestinationResource) path() string {
	return fmt.Sprintf("/observability/otlp/log-archive/%s", r.client.TenantID())
}

func (m *logArchiveDestinationModel) toBody() *logArchiveBody {
	return &logArchiveBody{
		Type:            m.Type.ValueString(),
		URL:             strPtr(m.SasURL),
		Bucket:          strPtr(m.Bucket),
		Region:          strPtr(m.Region),
		Endpoint:        strPtr(m.Endpoint),
		AccessKeyID:     strPtr(m.AccessKeyID),
		SecretAccessKey: strPtr(m.SecretAccessKey),
		Enabled:         boolPtr(m.Enabled),
	}
}

func (m *logArchiveDestinationModel) applyResponse(tenantID string, in *logArchiveDestinationResponse) {
	m.ID = types.StringValue(tenantID)
	m.Type = types.StringValue(in.Type)
	m.Region = strFromPtr(in.Region)
	m.Enabled = types.BoolValue(in.Enabled)
	m.BaseURL = types.StringValue(in.BaseURL)
}

func (r *logArchiveDestinationResource) put(ctx context.Context, plan *logArchiveDestinationModel, ds *diagSink) {
	var got logArchiveDestinationResponse
	if err := r.client.Put(ctx, r.path(), plan.toBody(), &got); err != nil {
		ds.d.AddError("Error saving log archive destination", err.Error())
		return
	}
	plan.applyResponse(r.client.TenantID(), &got)
}

func (r *logArchiveDestinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan logArchiveDestinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.put(ctx, &plan, &diagSink{d: &resp.Diagnostics}); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *logArchiveDestinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state logArchiveDestinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got struct {
		Destination *logArchiveDestinationResponse `json:"destination"`
	}
	if err := r.client.Get(ctx, r.path(), nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading log archive destination", err.Error())
		return
	}
	if got.Destination == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(r.client.TenantID(), got.Destination)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *logArchiveDestinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan logArchiveDestinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.put(ctx, &plan, &diagSink{d: &resp.Diagnostics}); resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *logArchiveDestinationResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	if err := r.client.Delete(ctx, r.path()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting log archive destination", err.Error())
	}
}

func (r *logArchiveDestinationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
