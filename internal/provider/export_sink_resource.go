package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A data export sink (Monitoring, /export-sinks): a daily copy of monitoring
// data to the tenant's own Azure Blob container or S3 bucket. No GET by id, so
// Read lists and selects. The credential is write-only.

var (
	_ resource.Resource                = (*exportSinkResource)(nil)
	_ resource.ResourceWithConfigure   = (*exportSinkResource)(nil)
	_ resource.ResourceWithImportState = (*exportSinkResource)(nil)
)

var exportDatasets = []string{"check_results", "monitor_hourly", "monitors", "alert_events", "incidents"}

// NewExportSinkResource is the resource constructor wired into the provider.
func NewExportSinkResource() resource.Resource { return &exportSinkResource{} }

type exportSinkResource struct {
	client *client.Client
}

type exportSinkModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Kind            types.String `tfsdk:"kind"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Format          types.String `tfsdk:"format"`
	Prefix          types.String `tfsdk:"prefix"`
	Datasets        types.Set    `tfsdk:"datasets"`
	EnvironmentIDs  types.Set    `tfsdk:"environment_ids"`
	ScheduleHourUTC types.Int64  `tfsdk:"schedule_hour_utc"`
	AccountURL      types.String `tfsdk:"account_url"`
	Container       types.String `tfsdk:"container"`
	Endpoint        types.String `tfsdk:"endpoint"`
	Bucket          types.String `tfsdk:"bucket"`
	Region          types.String `tfsdk:"region"`
	ForcePathStyle  types.Bool   `tfsdk:"force_path_style"`
	Secret          types.String `tfsdk:"secret"`
	SecretHint      types.String `tfsdk:"secret_hint"`
}

type exportSinkConfig struct {
	AccountURL     *string `json:"accountUrl,omitempty"`
	Container      *string `json:"container,omitempty"`
	Endpoint       *string `json:"endpoint,omitempty"`
	Bucket         *string `json:"bucket,omitempty"`
	Region         *string `json:"region,omitempty"`
	ForcePathStyle *bool   `json:"forcePathStyle,omitempty"`
}

type exportSinkBody struct {
	Name            string           `json:"name"`
	Kind            string           `json:"kind"`
	Enabled         *bool            `json:"enabled,omitempty"`
	Format          *string          `json:"format,omitempty"`
	Prefix          *string          `json:"prefix,omitempty"`
	Datasets        []string         `json:"datasets"`
	EnvironmentIDs  []string         `json:"environmentIds"`
	ScheduleHourUTC *int64           `json:"scheduleHourUtc,omitempty"`
	Config          exportSinkConfig `json:"config"`
	Secret          string           `json:"secret"`
}

type exportSinkResponse struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Kind            string           `json:"kind"`
	Enabled         bool             `json:"enabled"`
	Format          string           `json:"format"`
	Prefix          string           `json:"prefix"`
	Datasets        []string         `json:"datasets"`
	EnvironmentIDs  []string         `json:"environmentIds"`
	Config          exportSinkConfig `json:"config"`
	SecretHint      string           `json:"secretHint"`
	ScheduleHourUTC int64            `json:"scheduleHourUtc"`
}

func (r *exportSinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_export_sink"
}

func (r *exportSinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *exportSinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{MarkdownDescription: desc, Optional: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Exports monitoring data every day to your own Azure Blob container (`kind = \"azure_blob\"`, " +
			"set `account_url` and `container`) or S3 bucket (`kind = \"s3\"`, set `bucket`, `region` and, for an " +
			"S3-compatible store, `endpoint`). Files land at `<prefix>/<dataset>/dt=<date>/`. Requires the data " +
			"export sinks feature and `settings.manage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Sink ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name (120 characters max).",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 120)},
			},
			"kind": schema.StringAttribute{
				MarkdownDescription: "`azure_blob` or `s3`. Changing it replaces the sink.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.OneOf("azure_blob", "s3")},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Run the daily export. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"format": schema.StringAttribute{
				MarkdownDescription: "`ndjson` (gzip) or `csv`. Defaults to `ndjson`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("ndjson"),
				Validators:          []validator.String{stringvalidator.OneOf("ndjson", "csv")},
			},
			"prefix": schema.StringAttribute{
				MarkdownDescription: "Path prefix inside the container or bucket. Defaults to `watchpoint`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("watchpoint"),
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 256)},
			},
			"datasets": schema.SetAttribute{
				MarkdownDescription: "Datasets to export: `check_results`, `monitor_hourly`, `monitors`, `alert_events`, `incidents`.",
				Required:            true,
				ElementType:         types.StringType,
				Validators:          []validator.Set{setvalidator.SizeAtLeast(1), setvalidator.ValueStringsAre(stringvalidator.OneOf(exportDatasets...))},
			},
			"environment_ids": schema.SetAttribute{
				MarkdownDescription: "Limit the export to these environments (`watchpoint_environment` ids). Omit for every environment.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"schedule_hour_utc": schema.Int64Attribute{
				MarkdownDescription: "UTC hour (0 to 23) the daily export runs. Defaults to `2`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(2),
				Validators:          []validator.Int64{int64validator.Between(0, 23)},
			},
			"account_url": optStr("Azure Blob: the storage account URL, such as `https://account.blob.core.windows.net`."),
			"container":   optStr("Azure Blob: the container name."),
			"endpoint":    optStr("S3: the https endpoint of an S3-compatible store. Omit for AWS."),
			"bucket":      optStr("S3: the bucket name."),
			"region":      optStr("S3: the bucket region, such as `us-east-1`."),
			"force_path_style": schema.BoolAttribute{
				MarkdownDescription: "S3: use path-style addressing, which most S3-compatible stores need. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"secret": schema.StringAttribute{
				MarkdownDescription: "Credential. Azure Blob: a SAS token or connection string. S3: " +
					"`accessKeyId:secretAccessKey`. Write-only: only its last four characters are returned " +
					"(`secret_hint`), so a credential changed outside Terraform is not detected.",
				Required:  true,
				Sensitive: true,
			},
			"secret_hint": schema.StringAttribute{
				MarkdownDescription: "Last four characters of the stored credential.",
				Computed:            true,
			},
		},
	}
}

func (m *exportSinkModel) toBody(ctx context.Context, ds *diagSink) *exportSinkBody {
	b := &exportSinkBody{
		Name:            m.Name.ValueString(),
		Kind:            m.Kind.ValueString(),
		Enabled:         boolPtr(m.Enabled),
		Format:          strPtr(m.Format),
		Prefix:          strPtr(m.Prefix),
		Datasets:        stringSliceFromSet(ctx, m.Datasets, ds.d),
		EnvironmentIDs:  emptyIfNil(stringSliceFromSet(ctx, m.EnvironmentIDs, ds.d)),
		ScheduleHourUTC: int64Ptr(m.ScheduleHourUTC),
		Secret:          m.Secret.ValueString(),
	}
	if b.Kind == "azure_blob" {
		b.Config = exportSinkConfig{AccountURL: strPtr(m.AccountURL), Container: strPtr(m.Container)}
	} else {
		b.Config = exportSinkConfig{Endpoint: strPtr(m.Endpoint), Bucket: strPtr(m.Bucket), Region: strPtr(m.Region), ForcePathStyle: boolPtr(m.ForcePathStyle)}
	}
	return b
}

func (m *exportSinkModel) applyResponse(ctx context.Context, in *exportSinkResponse, ds *diagSink) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.Kind = types.StringValue(in.Kind)
	m.Enabled = types.BoolValue(in.Enabled)
	m.Format = types.StringValue(in.Format)
	m.Prefix = types.StringValue(in.Prefix)
	m.Datasets = setFromStringSlice(ctx, in.Datasets, ds.d)
	if len(in.EnvironmentIDs) == 0 {
		m.EnvironmentIDs = types.SetNull(types.StringType)
	} else {
		m.EnvironmentIDs = setFromStringSlice(ctx, in.EnvironmentIDs, ds.d)
	}
	m.ScheduleHourUTC = types.Int64Value(in.ScheduleHourUTC)
	m.AccountURL = strFromPtr(in.Config.AccountURL)
	m.Container = strFromPtr(in.Config.Container)
	m.Endpoint = strFromPtr(in.Config.Endpoint)
	m.Bucket = strFromPtr(in.Config.Bucket)
	m.Region = strFromPtr(in.Config.Region)
	m.ForcePathStyle = types.BoolValue(in.Config.ForcePathStyle != nil && *in.Config.ForcePathStyle)
	m.SecretHint = types.StringValue(in.SecretHint)
}

func (r *exportSinkResource) path(suffix string) string {
	return r.client.Tenantf("/monitoring", "/export-sinks%s", suffix)
}

func (r *exportSinkResource) find(ctx context.Context, id string) (*exportSinkResponse, error) {
	var out struct {
		Items []exportSinkResponse `json:"items"`
	}
	if err := r.client.Get(ctx, r.path(""), nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Items {
		if out.Items[i].ID == id {
			return &out.Items[i], nil
		}
	}
	return nil, nil
}

func (r *exportSinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan exportSinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}
	var got exportSinkResponse
	if err := r.client.Post(ctx, r.path(""), body, &got); err != nil {
		resp.Diagnostics.AddError("Error creating export sink", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *exportSinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state exportSinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.find(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading export sink", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(ctx, got, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update resends the credential with the config: the API refuses a destination change without it.
func (r *exportSinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan exportSinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}
	var got exportSinkResponse
	if err := r.client.Patch(ctx, r.path("/"+plan.ID.ValueString()), body, &got); err != nil {
		resp.Diagnostics.AddError("Error updating export sink", err.Error())
		return
	}
	plan.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *exportSinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state exportSinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path("/"+state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting export sink", err.Error())
	}
}

func (r *exportSinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
