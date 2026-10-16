package provider

import (
	"context"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// status_page_external_source points one status-page component at somebody
// else's status page, so a dependency you do not run still shows up on yours.
// Owned by the Status API (Watchpoint.Status.Api ExternalComponentSourcesController)
// and gated on the `status_third_party_components` feature flag.
//
// A component has at most one source, so the endpoint is a PUT upsert keyed on
// the component and there is no separate create. component_id and status_page_id
// therefore identify the resource and both force replacement; the id in state is
// the source row's own id, and the import id is "<status_page_id>/<component_id>".
//
// There is no GET for a single source, so Read lists the page's sources and
// picks ours out by component.
//
// poll_interval_seconds is validated against the API's own 60-3600 clamp rather
// than left to the server: a value outside the range is silently clamped, which
// would come back as a value the configuration never asked for and fail the
// apply.
//
// Changing url, kind or json_path re-targets the source, which clears the cached
// reading server-side. That is a normal update, not a replacement.

var (
	_ resource.Resource                = (*statusPageExternalSourceResource)(nil)
	_ resource.ResourceWithConfigure   = (*statusPageExternalSourceResource)(nil)
	_ resource.ResourceWithImportState = (*statusPageExternalSourceResource)(nil)
)

// externalStatusSourceKinds are the ExternalStatusSourceKind names the API accepts.
var externalStatusSourceKinds = []string{"StatuspageSummary", "HttpJson"}

// NewStatusPageExternalSourceResource is the resource constructor wired into the provider.
func NewStatusPageExternalSourceResource() resource.Resource {
	return &statusPageExternalSourceResource{}
}

type statusPageExternalSourceResource struct {
	client *client.Client
}

// statusPageExternalSourceModel maps schema attributes <-> Terraform state.
type statusPageExternalSourceModel struct {
	ID                 types.String `tfsdk:"id"`
	StatusPageID       types.String `tfsdk:"status_page_id"`
	ComponentID        types.String `tfsdk:"component_id"`
	Kind               types.String `tfsdk:"kind"`
	URL                types.String `tfsdk:"url"`
	JSONPath           types.String `tfsdk:"json_path"`
	OperationalValues  types.String `tfsdk:"operational_values"`
	DegradedValues     types.String `tfsdk:"degraded_values"`
	OutageValues       types.String `tfsdk:"outage_values"`
	PollIntervalSecond types.Int64  `tfsdk:"poll_interval_seconds"`
	IsEnabled          types.Bool   `tfsdk:"is_enabled"`
	EffectiveHealth    types.String `tfsdk:"effective_health"`
	CreatedAt          types.String `tfsdk:"created_at"`
	UpdatedAt          types.String `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the status API) ---

// statusPageExternalSourceBody mirrors UpsertRequest. The PUT replaces the whole
// row, so every field is sent on every write and a nil is a real "unset".
type statusPageExternalSourceBody struct {
	Kind                string  `json:"kind"`
	URL                 string  `json:"url"`
	JSONPath            *string `json:"jsonPath,omitempty"`
	OperationalValues   *string `json:"operationalValues,omitempty"`
	DegradedValues      *string `json:"degradedValues,omitempty"`
	OutageValues        *string `json:"outageValues,omitempty"`
	PollIntervalSeconds *int64  `json:"pollIntervalSeconds,omitempty"`
	IsEnabled           *bool   `json:"isEnabled,omitempty"`
}

type statusPageExternalSourceResponse struct {
	ID                  string  `json:"id"`
	ComponentID         string  `json:"componentId"`
	StatusPageID        string  `json:"statusPageId"`
	Kind                string  `json:"kind"`
	URL                 string  `json:"url"`
	JSONPath            *string `json:"jsonPath"`
	OperationalValues   *string `json:"operationalValues"`
	DegradedValues      *string `json:"degradedValues"`
	OutageValues        *string `json:"outageValues"`
	PollIntervalSeconds int64   `json:"pollIntervalSeconds"`
	IsEnabled           bool    `json:"isEnabled"`
	EffectiveHealth     string  `json:"effectiveHealth"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
}

func (r *statusPageExternalSourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_external_source"
}

func (r *statusPageExternalSourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *statusPageExternalSourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Drives one status page component from a third-party status page instead of from " +
			"your own monitors, so an upstream provider you depend on appears on your page. Requires the " +
			"`status_third_party_components` feature flag. A component has at most one source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "External source ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status_page_id": schema.StringAttribute{
				MarkdownDescription: "ID of the status page the component belongs to. Changing it replaces the source.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"component_id": schema.StringAttribute{
				MarkdownDescription: "ID of the component this source drives. Changing it replaces the source.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"kind": schema.StringAttribute{
				MarkdownDescription: "How the upstream status is read: `StatuspageSummary` reads " +
					"`status.indicator` from an Atlassian Statuspage `/api/v2/summary.json`, `HttpJson` reads " +
					"any JSON endpoint at `json_path`.",
				Required:   true,
				Validators: []validator.String{stringvalidator.OneOf(externalStatusSourceKinds...)},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "Absolute HTTPS URL to poll. Rejected if it resolves to a private or " +
					"link-local address, and re-checked at poll time.",
				Required:   true,
				Validators: []validator.String{stringvalidator.LengthBetween(1, 2048)},
			},
			"json_path": schema.StringAttribute{
				MarkdownDescription: "`HttpJson` only, and required for it: dotted path to the field to read, " +
					"with optional numeric array indexes, for example `status.indicator` or `components.0.state`. " +
					"Ignored for `StatuspageSummary`.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.LengthBetween(1, 200)},
			},
			"operational_values": schema.StringAttribute{
				MarkdownDescription: "Comma-separated values at `json_path` that mean healthy, for example " +
					"`none,operational`. Case-insensitive. Omit to use the built-in vocabulary.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.LengthBetween(1, 500)},
			},
			"degraded_values": schema.StringAttribute{
				MarkdownDescription: "Comma-separated values that mean degraded, for example `minor,degraded_performance`.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 500)},
			},
			"outage_values": schema.StringAttribute{
				MarkdownDescription: "Comma-separated values that mean an outage, for example `critical,major_outage`.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 500)},
			},
			"poll_interval_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds between polls, between 60 and 3600. Defaults to `300`, because a " +
					"third-party status page moves in minutes and a faster poll is traffic they will notice.",
				Optional:   true,
				Computed:   true,
				Default:    int64default.StaticInt64(300),
				Validators: []validator.Int64{int64validator.Between(60, 3600)},
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the source is polled. Defaults to `true`. A disabled source leaves " +
					"the component reading `Unknown`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"effective_health": schema.StringAttribute{
				MarkdownDescription: "What the public page paints for this component right now (`Unknown`, " +
					"`Operational`, `Degraded`, `PartialOutage`, `MajorOutage`). `Unknown` until the first poll " +
					"lands, and again once a reading goes stale.",
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "Last-update timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *statusPageExternalSourceModel) toBody() *statusPageExternalSourceBody {
	return &statusPageExternalSourceBody{
		Kind:                m.Kind.ValueString(),
		URL:                 m.URL.ValueString(),
		JSONPath:            strPtr(m.JSONPath),
		OperationalValues:   strPtr(m.OperationalValues),
		DegradedValues:      strPtr(m.DegradedValues),
		OutageValues:        strPtr(m.OutageValues),
		PollIntervalSeconds: int64Ptr(m.PollIntervalSecond),
		IsEnabled:           boolPtr(m.IsEnabled),
	}
}

// findStatusPageExternalSource picks this component's source out of a page
// listing, or returns nil when it is gone. Keyed on the component, not the row
// id: one component has at most one source, and that is what the PUT addresses.
func findStatusPageExternalSource(sources []statusPageExternalSourceResponse, componentID string) *statusPageExternalSourceResponse {
	for i := range sources {
		if sources[i].ComponentID == componentID {
			return &sources[i]
		}
	}
	return nil
}

// applyResponse folds the API response into state. json_path is dropped by the
// API for a StatuspageSummary source, so it comes back null there whatever the
// configuration said.
func (m *statusPageExternalSourceModel) applyResponse(resp *statusPageExternalSourceResponse) {
	m.ID = types.StringValue(resp.ID)
	m.StatusPageID = types.StringValue(resp.StatusPageID)
	m.ComponentID = types.StringValue(resp.ComponentID)
	m.Kind = types.StringValue(resp.Kind)
	m.URL = types.StringValue(resp.URL)
	m.JSONPath = strFromPtr(resp.JSONPath)
	m.OperationalValues = strFromPtr(resp.OperationalValues)
	m.DegradedValues = strFromPtr(resp.DegradedValues)
	m.OutageValues = strFromPtr(resp.OutageValues)
	m.PollIntervalSecond = types.Int64Value(resp.PollIntervalSeconds)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.EffectiveHealth = types.StringValue(resp.EffectiveHealth)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
}

// upsert is the whole write path: the API models a source as a PUT on its
// component, so create and update are the same call.
func (r *statusPageExternalSourceResource) upsert(ctx context.Context, m *statusPageExternalSourceModel) error {
	path := r.client.Tenantf("/status", "/pages/%s/components/%s/external-source",
		m.StatusPageID.ValueString(), m.ComponentID.ValueString())
	var got statusPageExternalSourceResponse
	if err := r.client.Put(ctx, path, m.toBody(), &got); err != nil {
		return err
	}
	m.applyResponse(&got)
	return nil
}

func (r *statusPageExternalSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageExternalSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.upsert(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error creating status page external source", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageExternalSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageExternalSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var sources []statusPageExternalSourceResponse
	listPath := r.client.Tenantf("/status", "/pages/%s/external-sources", state.StatusPageID.ValueString())
	if err := r.client.Get(ctx, listPath, nil, &sources); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading status page external source", err.Error())
		return
	}

	got := findStatusPageExternalSource(sources, state.ComponentID.ValueString())
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.applyResponse(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageExternalSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageExternalSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.upsert(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error updating status page external source", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageExternalSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageExternalSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/status", "/pages/%s/components/%s/external-source",
		state.StatusPageID.ValueString(), state.ComponentID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting status page external source", err.Error())
	}
}

// ImportState takes "<status_page_id>/<component_id>" rather than the source id,
// because that pair is what every endpoint addresses. Read fills in the rest.
func (r *statusPageExternalSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the form \"<status_page_id>/<component_id>\", got: "+req.ID,
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("component_id"), parts[1])...)
}
