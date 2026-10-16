package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// rum_site maps onto the Watchpoint RUM API's sites. The PublicKey is embedded
// in the customer's <script> tag and is server-generated.

var (
	_ resource.Resource                = (*rumSiteResource)(nil)
	_ resource.ResourceWithConfigure   = (*rumSiteResource)(nil)
	_ resource.ResourceWithImportState = (*rumSiteResource)(nil)
)

// NewRUMSiteResource is the resource constructor wired into the provider.
func NewRUMSiteResource() resource.Resource { return &rumSiteResource{} }

type rumSiteResource struct {
	client *client.Client
}

// rumSiteModel maps schema attributes <-> Terraform state. additionalOrigins
// and piiMaskSelectors are CSV strings in the DTO, so they stay types.String.
type rumSiteModel struct {
	ID                       types.String  `tfsdk:"id"`
	Name                     types.String  `tfsdk:"name"`
	PrimaryOrigin            types.String  `tfsdk:"primary_origin"`
	AdditionalOrigins        types.String  `tfsdk:"additional_origins"`
	PublicKey                types.String  `tfsdk:"public_key"`
	SampleRate               types.Float64 `tfsdk:"sample_rate"`
	ResourceTimingSampleRate types.Float64 `tfsdk:"resource_timing_sample_rate"`
	IsEnabled                types.Bool    `tfsdk:"is_enabled"`
	ReplayEnabled            types.Bool    `tfsdk:"replay_enabled"`
	ReplaySampleRate         types.Float64 `tfsdk:"replay_sample_rate"`
	HeatmapsEnabled          types.Bool    `tfsdk:"heatmaps_enabled"`
	PiiMaskSelectors         types.String  `tfsdk:"pii_mask_selectors"`
	ScreenshotAuthUsername   types.String  `tfsdk:"screenshot_auth_username"`
	ScreenshotAuthPassVault  types.String  `tfsdk:"screenshot_auth_password_vault_key"`
	InstallSnippet           types.String  `tfsdk:"install_snippet"`
	CreatedAt                types.String  `tfsdk:"created_at"`
	UpdatedAt                types.String  `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the RUM API) ---

// rumSiteCreateBody mirrors CreateRumSiteRequest.
type rumSiteCreateBody struct {
	Name                     string   `json:"name"`
	PrimaryOrigin            string   `json:"primaryOrigin"`
	AdditionalOrigins        *string  `json:"additionalOrigins,omitempty"`
	SampleRate               *float64 `json:"sampleRate,omitempty"`
	ResourceTimingSampleRate *float64 `json:"resourceTimingSampleRate,omitempty"`
}

// rumSiteUpdateBody mirrors UpdateRumSiteRequest (PUT).
type rumSiteUpdateBody struct {
	Name                     *string  `json:"name,omitempty"`
	PrimaryOrigin            *string  `json:"primaryOrigin,omitempty"`
	AdditionalOrigins        *string  `json:"additionalOrigins"`
	SampleRate               *float64 `json:"sampleRate,omitempty"`
	ResourceTimingSampleRate *float64 `json:"resourceTimingSampleRate,omitempty"`
	IsEnabled                *bool    `json:"isEnabled,omitempty"`
	ReplayEnabled            *bool    `json:"replayEnabled,omitempty"`
	ReplaySampleRate         *float64 `json:"replaySampleRate,omitempty"`
	HeatmapsEnabled          *bool    `json:"heatmapsEnabled,omitempty"`
	PiiMaskSelectors         *string  `json:"piiMaskSelectors,omitempty"`
	ScreenshotAuthUsername   *string  `json:"screenshotAuthUsername"`
	ScreenshotAuthPassVault  *string  `json:"screenshotAuthPasswordVaultKey"`
}

// rumSiteResponse mirrors RumSiteResponse.
type rumSiteResponse struct {
	ID                       string  `json:"id"`
	TenantID                 string  `json:"tenantId"`
	Name                     string  `json:"name"`
	PrimaryOrigin            string  `json:"primaryOrigin"`
	AdditionalOrigins        *string `json:"additionalOrigins"`
	PublicKey                string  `json:"publicKey"`
	SampleRate               float64 `json:"sampleRate"`
	ResourceTimingSampleRate float64 `json:"resourceTimingSampleRate"`
	IsEnabled                bool    `json:"isEnabled"`
	ReplayEnabled            bool    `json:"replayEnabled"`
	ReplaySampleRate         float64 `json:"replaySampleRate"`
	HeatmapsEnabled          bool    `json:"heatmapsEnabled"`
	PiiMaskSelectors         *string `json:"piiMaskSelectors"`
	ScreenshotAuthUsername   *string `json:"screenshotAuthUsername"`
	ScreenshotAuthPassVault  *string `json:"screenshotAuthPasswordVaultKey"`
	CreatedAt                string  `json:"createdAt"`
	UpdatedAt                string  `json:"updatedAt"`
	InstallSnippet           string  `json:"installSnippet"`
}

func (r *rumSiteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rum_site"
}

func (r *rumSiteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *rumSiteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Real User Monitoring (RUM) site. The generated public key is embedded in " +
			"the customer's snippet; rotating it (out of band) breaks deployed snippets.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "RUM site ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable site name.",
				Required:            true,
			},
			"primary_origin": schema.StringAttribute{
				MarkdownDescription: "Primary site origin (absolute http(s) URL). Normalized to scheme + authority server-side.",
				Required:            true,
			},
			"additional_origins": schema.StringAttribute{
				MarkdownDescription: "Comma-separated additional allowed origins (stored as a CSV string).",
				Optional:            true,
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "Server-generated public key embedded in the install snippet.",
				Computed:            true,
			},
			"sample_rate": schema.Float64Attribute{
				MarkdownDescription: "Page-view sampling rate in [0,1]. Defaults to `1.0`.",
				Optional:            true,
				Computed:            true,
			},
			"resource_timing_sample_rate": schema.Float64Attribute{
				MarkdownDescription: "Resource-timing sampling rate in [0,1]. Defaults to `0.1`.",
				Optional:            true,
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the site is collecting. Defaults to `true` on create.",
				Optional:            true,
				Computed:            true,
			},
			"replay_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether session replay is enabled.",
				Optional:            true,
				Computed:            true,
			},
			"replay_sample_rate": schema.Float64Attribute{
				MarkdownDescription: "Session-replay sampling rate in [0,1].",
				Optional:            true,
				Computed:            true,
			},
			"heatmaps_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether heatmap capture is enabled.",
				Optional:            true,
				Computed:            true,
			},
			"pii_mask_selectors": schema.StringAttribute{
				MarkdownDescription: "Comma-separated CSS selectors whose content is masked (stored as a CSV string).",
				Optional:            true,
				Computed:            true,
			},
			"screenshot_auth_username": schema.StringAttribute{
				MarkdownDescription: "Basic-auth username the heatmap screenshot capture uses to reach pages " +
					"behind a login. Set it together with `screenshot_auth_password_vault_key`; without " +
					"both, capture requests go out unauthenticated and a gated page renders as its login " +
					"screen. Removing it from configuration clears it.",
				Optional: true,
			},
			"screenshot_auth_password_vault_key": schema.StringAttribute{
				MarkdownDescription: "Key of the `watchpoint_vault_secret` holding the screenshot basic-auth " +
					"password. Only the key is stored on the site; the password itself stays in the vault " +
					"and is resolved at capture time. Removing it from configuration clears it.",
				Optional: true,
			},
			"install_snippet": schema.StringAttribute{
				MarkdownDescription: "Ready-to-paste <script> install snippet (contains the public key).",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "Last-updated timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func (m *rumSiteModel) toCreateBody() *rumSiteCreateBody {
	return &rumSiteCreateBody{
		Name:                     m.Name.ValueString(),
		PrimaryOrigin:            m.PrimaryOrigin.ValueString(),
		AdditionalOrigins:        strPtr(m.AdditionalOrigins),
		SampleRate:               float64Ptr(m.SampleRate),
		ResourceTimingSampleRate: float64Ptr(m.ResourceTimingSampleRate),
	}
}

func (m *rumSiteModel) toUpdateBody() *rumSiteUpdateBody {
	return &rumSiteUpdateBody{
		Name:          strPtr(m.Name),
		PrimaryOrigin: strPtr(m.PrimaryOrigin),
		// Plain Optional: send "" so dropping it from config clears it.
		AdditionalOrigins:        strPtrClearing(m.AdditionalOrigins),
		SampleRate:               float64Ptr(m.SampleRate),
		ResourceTimingSampleRate: float64Ptr(m.ResourceTimingSampleRate),
		IsEnabled:                boolPtr(m.IsEnabled),
		ReplayEnabled:            boolPtr(m.ReplayEnabled),
		ReplaySampleRate:         float64Ptr(m.ReplaySampleRate),
		HeatmapsEnabled:          boolPtr(m.HeatmapsEnabled),
		PiiMaskSelectors:         strPtr(m.PiiMaskSelectors),
		// Plain Optional, and the API reads "" as "clear" and null as "leave
		// alone", so send "" to make removal from config actually take effect.
		ScreenshotAuthUsername:  strPtrClearing(m.ScreenshotAuthUsername),
		ScreenshotAuthPassVault: strPtrClearing(m.ScreenshotAuthPassVault),
	}
}

func (m *rumSiteModel) applyResponse(resp *rumSiteResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.PrimaryOrigin = types.StringValue(resp.PrimaryOrigin)
	m.AdditionalOrigins = strFromPtr(resp.AdditionalOrigins)
	m.PublicKey = types.StringValue(resp.PublicKey)
	m.SampleRate = types.Float64Value(resp.SampleRate)
	m.ResourceTimingSampleRate = types.Float64Value(resp.ResourceTimingSampleRate)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.ReplayEnabled = types.BoolValue(resp.ReplayEnabled)
	m.ReplaySampleRate = types.Float64Value(resp.ReplaySampleRate)
	m.HeatmapsEnabled = types.BoolValue(resp.HeatmapsEnabled)
	m.PiiMaskSelectors = strFromPtr(resp.PiiMaskSelectors)
	m.ScreenshotAuthUsername = strFromPtr(resp.ScreenshotAuthUsername)
	m.ScreenshotAuthPassVault = strFromPtr(resp.ScreenshotAuthPassVault)
	m.InstallSnippet = types.StringValue(resp.InstallSnippet)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
}

func (r *rumSiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan rumSiteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created rumSiteResponse
	path := r.client.Tenantf("/rum", "/sites")
	if err := r.client.Post(ctx, path, plan.toCreateBody(), &created); err != nil {
		resp.Diagnostics.AddError("Error creating RUM site", err.Error())
		return
	}

	// Create accepts only name/origin/sample-rates. If the plan set any
	// update-only toggles, push them with a follow-up PUT.
	if hasRumUpdateOnly(&plan) {
		var updated rumSiteResponse
		putPath := r.client.Tenantf("/rum", "/sites/%s", created.ID)
		if err := r.client.Put(ctx, putPath, plan.toUpdateBody(), &updated); err != nil {
			resp.Diagnostics.AddError("Error applying settings after create", err.Error())
			return
		}
		plan.applyResponse(&updated)
	} else {
		plan.applyResponse(&created)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func hasRumUpdateOnly(m *rumSiteModel) bool {
	return !m.IsEnabled.IsNull() || !m.ReplayEnabled.IsNull() || !m.ReplaySampleRate.IsNull() ||
		!m.HeatmapsEnabled.IsNull() || !m.PiiMaskSelectors.IsNull() ||
		!m.ScreenshotAuthUsername.IsNull() || !m.ScreenshotAuthPassVault.IsNull()
}

func (r *rumSiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state rumSiteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got rumSiteResponse
	getPath := r.client.Tenantf("/rum", "/sites/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading RUM site", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *rumSiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan rumSiteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated rumSiteResponse
	path := r.client.Tenantf("/rum", "/sites/%s", plan.ID.ValueString())
	if err := r.client.Put(ctx, path, plan.toUpdateBody(), &updated); err != nil {
		resp.Diagnostics.AddError("Error updating RUM site", err.Error())
		return
	}

	plan.applyResponse(&updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *rumSiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state rumSiteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/rum", "/sites/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting RUM site", err.Error())
	}
}

func (r *rumSiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
