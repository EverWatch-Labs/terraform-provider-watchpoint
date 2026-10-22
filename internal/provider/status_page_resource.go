package provider

import (
	"context"
	"strings"
	"unicode"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// status_page maps onto the Watchpoint Status API's status pages. Page core CRUD,
// branding, the viewer default language and the private-page audience gate live
// here; components are their own resource (watchpoint_status_page_component) and
// subscribers/render endpoints are out of scope.
//
// CreatePageRequest accepts only slug/name/description/dashboard, so everything
// else is applied by a follow-up PATCH inside Create.
//
// access_password is write-only: the API hashes it and never echoes it back, so
// it is Optional + Sensitive and never Computed and never refreshed in Read.
// has_access_password is the only readable signal that a password is set.
//
// allowed_email_domains is an unordered membership set, so it is a Set and never
// a List. The API stores it as one normalised CSV string (lower-cased, leading
// "@" stripped, de-duplicated); applyResponse keeps the configured spelling when
// it normalises to the same set, because writing the normalised form back into a
// config-set attribute fails the apply.

// statusPageVisibilities are the StatusPageVisibility names the API accepts.
// TokenProtected is a reserved enum slot the API rejects with a 400 (no token
// gate is implemented), so it is deliberately absent.
var statusPageVisibilities = []string{"Public", "PasswordProtected", "EmailDomainRestricted"}

// statusPageLanguages mirrors StatusPagesController.SupportedLanguages, which is
// the catalogue the public viewer actually ships strings for. Anything else is a
// 400, so the list is pinned here and grows when the viewer's does.
var statusPageLanguages = []string{"en", "de", "fr", "es", "pt-BR", "ja", "nl"}

var (
	_ resource.Resource                = (*statusPageResource)(nil)
	_ resource.ResourceWithConfigure   = (*statusPageResource)(nil)
	_ resource.ResourceWithImportState = (*statusPageResource)(nil)
)

// NewStatusPageResource is the resource constructor wired into the provider.
func NewStatusPageResource() resource.Resource { return &statusPageResource{} }

type statusPageResource struct {
	client *client.Client
}

// statusPageModel maps schema attributes <-> Terraform state.
type statusPageModel struct {
	ID                  types.String `tfsdk:"id"`
	Slug                types.String `tfsdk:"slug"`
	TenantSlug          types.String `tfsdk:"tenant_slug"`
	TenantName          types.String `tfsdk:"tenant_name"`
	Name                types.String `tfsdk:"name"`
	Description         types.String `tfsdk:"description"`
	IsEnabled           types.Bool   `tfsdk:"is_enabled"`
	Visibility          types.String `tfsdk:"visibility"`
	PrimaryDashboardID  types.String `tfsdk:"primary_dashboard_id"`
	LogoURL             types.String `tfsdk:"logo_url"`
	FaviconURL          types.String `tfsdk:"favicon_url"`
	AccentColor         types.String `tfsdk:"accent_color"`
	FooterText          types.String `tfsdk:"footer_text"`
	FooterHTML          types.String `tfsdk:"footer_html"`
	DefaultLanguage     types.String `tfsdk:"default_language"`
	EnabledLanguages    types.Set    `tfsdk:"enabled_languages"`
	AccessPassword      types.String `tfsdk:"access_password"`
	HasAccessPassword   types.Bool   `tfsdk:"has_access_password"`
	AllowedEmailDomains types.Set    `tfsdk:"allowed_email_domains"`
	CustomCSS           types.String `tfsdk:"custom_css"`
	CreatedAt           types.String `tfsdk:"created_at"`
	UpdatedAt           types.String `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the status API) ---

// statusPageCreateBody mirrors CreatePageRequest.
type statusPageCreateBody struct {
	Slug               string  `json:"slug"`
	TenantSlug         string  `json:"tenantSlug"`
	TenantName         *string `json:"tenantName,omitempty"`
	Name               string  `json:"name"`
	Description        *string `json:"description,omitempty"`
	PrimaryDashboardID *string `json:"primaryDashboardId,omitempty"`
}

// statusPageUpdateBody mirrors PatchPageRequest.
type statusPageUpdateBody struct {
	Slug               *string `json:"slug,omitempty"`
	TenantSlug         *string `json:"tenantSlug,omitempty"`
	TenantName         *string `json:"tenantName,omitempty"`
	Name               *string `json:"name,omitempty"`
	Description        *string `json:"description,omitempty"`
	IsEnabled          *bool   `json:"isEnabled,omitempty"`
	PrimaryDashboardID *string `json:"primaryDashboardId,omitempty"`
	LogoURL            *string `json:"logoUrl,omitempty"`
	FaviconURL         *string `json:"faviconUrl,omitempty"`
	AccentColor        *string `json:"accentColor,omitempty"`
	FooterText         *string `json:"footerText,omitempty"`
	FooterHTML         *string `json:"footerHtml,omitempty"`
	// Always sent (never omitempty-skipped once non-nil): "" is how the API is
	// told to clear the setting, and nil would silently mean "leave unchanged",
	// which makes removing the attribute from config a no-op that then fails the
	// apply because the response still carries the old value.
	DefaultLanguage     *string  `json:"defaultLanguage,omitempty"`
	EnabledLanguages    []string `json:"enabledLanguages,omitempty"`
	Visibility          *string  `json:"visibility,omitempty"`
	AccessPassword      *string  `json:"accessPassword,omitempty"`
	AllowedEmailDomains *string  `json:"allowedEmailDomains,omitempty"`
	CustomCSS           *string  `json:"customCss,omitempty"`
}

// statusPageBranding mirrors the nested `branding` object in the Detail response.
type statusPageBranding struct {
	LogoURL     *string `json:"logoUrl"`
	FaviconURL  *string `json:"faviconUrl"`
	AccentColor *string `json:"accentColor"`
	FooterText  *string `json:"footerText"`
	FooterHTML  *string `json:"footerHtml"`
}

// statusPageResponse mirrors the Detail projection. Branding is nested.
type statusPageResponse struct {
	ID                  string              `json:"id"`
	Slug                string              `json:"slug"`
	TenantSlug          string              `json:"tenantSlug"`
	TenantName          *string             `json:"tenantName"`
	Name                string              `json:"name"`
	Description         *string             `json:"description"`
	IsEnabled           bool                `json:"isEnabled"`
	Visibility          string              `json:"visibility"`
	PrimaryDashboardID  *string             `json:"primaryDashboardId"`
	Branding            *statusPageBranding `json:"branding"`
	DefaultLanguage     *string             `json:"defaultLanguage"`
	EnabledLanguages    []string            `json:"enabledLanguages"`
	HasAccessPassword   bool                `json:"hasAccessPassword"`
	AllowedEmailDomains *string             `json:"allowedEmailDomains"`
	CustomCSS           *string             `json:"customCss"`
	CreatedAt           string              `json:"createdAt"`
	UpdatedAt           string              `json:"updatedAt"`
}

func (r *statusPageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
}

func (r *statusPageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *statusPageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A public status page for a tenant. Components, subscribers, and rendering " +
			"are managed outside this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Status page ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: "Per-tenant unique slug (3-63 lowercase alphanumerics / hyphens).",
				Required:            true,
			},
			"tenant_slug": schema.StringAttribute{
				MarkdownDescription: "Denormalized tenant slug used by the public host resolver (3-63 lowercase alphanumerics / hyphens).",
				Required:            true,
			},
			"tenant_name": schema.StringAttribute{
				MarkdownDescription: "Optional denormalized tenant display name.",
				Optional:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable status page name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional page description.",
				Optional:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the page is published. Defaults to the server default on create.",
				Optional:            true,
				Computed:            true,
			},
			"visibility": schema.StringAttribute{
				MarkdownDescription: "Audience gate on the page: `" + strings.Join(statusPageVisibilities, "`, `") +
					"`. Defaults to `Public`. Switching to `PasswordProtected` requires `access_password` and " +
					"switching to `EmailDomainRestricted` requires `allowed_email_domains`, in the same apply.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.OneOf(statusPageVisibilities...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"primary_dashboard_id": schema.StringAttribute{
				MarkdownDescription: "Optional dashboard whose components seed the page.",
				Optional:            true,
			},
			"logo_url": schema.StringAttribute{
				MarkdownDescription: "Branding: logo URL.",
				Optional:            true,
				Computed:            true,
			},
			"favicon_url": schema.StringAttribute{
				MarkdownDescription: "Branding: favicon URL.",
				Optional:            true,
				Computed:            true,
			},
			"accent_color": schema.StringAttribute{
				MarkdownDescription: "Branding: 6- or 8-digit hex accent color (e.g. `#3B82F6`).",
				Optional:            true,
				Computed:            true,
			},
			"footer_text": schema.StringAttribute{
				MarkdownDescription: "Branding: plain-text footer.",
				Optional:            true,
				Computed:            true,
			},
			"footer_html": schema.StringAttribute{
				MarkdownDescription: "Branding: HTML footer (sanitized server-side to a strict tag/attribute whitelist).",
				Optional:            true,
				Computed:            true,
			},
			"default_language": schema.StringAttribute{
				MarkdownDescription: "Language the public viewer renders in for every visitor: `" +
					strings.Join(statusPageLanguages, "`, `") + "`. Omit to follow the visitor's browser.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.OneOf(statusPageLanguages...)},
			},
			"enabled_languages": schema.SetAttribute{
				MarkdownDescription: "Languages the public viewer offers in its language picker. Must include " +
					"`default_language` when that is set. Omit to keep what the page has; reads return every " +
					"language when none are restricted. Restricting requires the status page localization feature.",
				Optional:      true,
				Computed:      true,
				ElementType:   types.StringType,
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				Validators:    []validator.Set{setvalidator.SizeAtLeast(1), setvalidator.ValueStringsAre(stringvalidator.OneOf(statusPageLanguages...))},
			},
			"access_password": schema.StringAttribute{
				MarkdownDescription: "Shared password visitors must enter when `visibility` is " +
					"`PasswordProtected`. Write-only: the API stores only a hash and never returns it, so " +
					"a password changed outside Terraform is not detected. Removing it from configuration " +
					"clears the stored password.",
				Optional:  true,
				Sensitive: true,
			},
			"has_access_password": schema.BoolAttribute{
				MarkdownDescription: "Whether an access password is currently stored. The password itself is never readable.",
				Computed:            true,
			},
			"allowed_email_domains": schema.SetAttribute{
				MarkdownDescription: "Email domains whose owners may view the page when `visibility` is " +
					"`EmailDomainRestricted`, for example `acme.com`. Unordered. Values are stored " +
					"lower-cased with any leading `@` stripped.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"custom_css": schema.StringAttribute{
				MarkdownDescription: "Custom stylesheet for the public page (20000 characters max). The API " +
					"sanitizes it and scopes every rule under the page root: `@import`, remote `url()`, " +
					"`expression()` and similar are stripped. The configured text is kept in state, so " +
					"sanitization does not show as drift. Requires the `status_custom_css` feature (Scale); " +
					"removing it is always allowed.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.LengthAtMost(20000)},
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

func (m *statusPageModel) toCreateBody() *statusPageCreateBody {
	return &statusPageCreateBody{
		Slug:               m.Slug.ValueString(),
		TenantSlug:         m.TenantSlug.ValueString(),
		TenantName:         strPtr(m.TenantName),
		Name:               m.Name.ValueString(),
		Description:        strPtr(m.Description),
		PrimaryDashboardID: strPtr(m.PrimaryDashboardID),
	}
}

// normalizeEmailDomains mirrors StatusPageAccessService.NormalizeDomainList:
// split on comma/semicolon/whitespace, lower-case, drop a leading "@",
// de-duplicate, keep first-seen order. Used both to build the request and to
// decide whether a stored list differs from configuration by more than that
// normalisation.
func normalizeEmailDomains(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, chunk := range raw {
		for _, part := range strings.FieldsFunc(chunk, func(r rune) bool {
			return r == ',' || r == ';' || unicode.IsSpace(r)
		}) {
			d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(part)), "@")
			if d == "" || seen[d] {
				continue
			}
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// sameEmailDomainSet reports whether two lists name the same domains once
// normalised. Order is irrelevant: this is a Set attribute.
func sameEmailDomainSet(a, b []string) bool {
	na, nb := normalizeEmailDomains(a), normalizeEmailDomains(b)
	if len(na) != len(nb) {
		return false
	}
	seen := map[string]bool{}
	for _, d := range na {
		seen[d] = true
	}
	for _, d := range nb {
		if !seen[d] {
			return false
		}
	}
	return true
}

func (m *statusPageModel) toUpdateBody(ctx context.Context, diags *diagSink) *statusPageUpdateBody {
	// Always a non-nil pointer so that removing the attribute from config sends
	// "" (clear) rather than omitting the field (leave unchanged).
	domains := strings.Join(normalizeEmailDomains(stringSliceFromSet(ctx, m.AllowedEmailDomains, diags.d)), ",")
	return &statusPageUpdateBody{
		DefaultLanguage:     strPtrClearing(m.DefaultLanguage),
		EnabledLanguages:    stringSliceFromSet(ctx, m.EnabledLanguages, diags.d),
		Visibility:          strPtr(m.Visibility),
		AccessPassword:      strPtr(m.AccessPassword),
		AllowedEmailDomains: &domains,
		Slug:                strPtr(m.Slug),
		TenantSlug:          strPtr(m.TenantSlug),
		TenantName:          strPtr(m.TenantName),
		Name:                strPtr(m.Name),
		Description:         strPtr(m.Description),
		IsEnabled:           boolPtr(m.IsEnabled),
		PrimaryDashboardID:  strPtr(m.PrimaryDashboardID),
		LogoURL:             strPtr(m.LogoURL),
		FaviconURL:          strPtr(m.FaviconURL),
		AccentColor:         strPtr(m.AccentColor),
		FooterText:          strPtr(m.FooterText),
		FooterHTML:          strPtr(m.FooterHTML),
		CustomCSS:           strPtrClearing(m.CustomCSS),
	}
}

// applyResponse folds the API response into state. access_password is never
// touched: the API does not return it, so the configured value stays put.
func (m *statusPageModel) applyResponse(ctx context.Context, resp *statusPageResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Slug = types.StringValue(resp.Slug)
	m.TenantSlug = types.StringValue(resp.TenantSlug)
	m.TenantName = strFromPtr(resp.TenantName)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.Visibility = types.StringValue(resp.Visibility)
	m.PrimaryDashboardID = strFromPtr(resp.PrimaryDashboardID)
	if resp.Branding != nil {
		m.LogoURL = strFromPtr(resp.Branding.LogoURL)
		m.FaviconURL = strFromPtr(resp.Branding.FaviconURL)
		m.AccentColor = strFromPtr(resp.Branding.AccentColor)
		m.FooterText = strFromPtr(resp.Branding.FooterText)
		m.FooterHTML = strFromPtr(resp.Branding.FooterHTML)
	} else {
		m.LogoURL = types.StringNull()
		m.FaviconURL = types.StringNull()
		m.AccentColor = types.StringNull()
		m.FooterText = types.StringNull()
		m.FooterHTML = types.StringNull()
	}
	m.DefaultLanguage = strFromPtr(resp.DefaultLanguage)
	m.EnabledLanguages = setFromStringSlice(ctx, resp.EnabledLanguages, diags.d)
	m.HasAccessPassword = types.BoolValue(resp.HasAccessPassword)
	// Keep the configured spelling when the stored list is the same set of
	// domains: the API lower-cases and strips "@", and writing that normalised
	// form back into a config-set attribute fails the apply. A list someone
	// actually changed elsewhere is real drift and does land in state.
	stored := normalizeEmailDomains([]string{deref(resp.AllowedEmailDomains)})
	configured := stringSliceFromSet(ctx, m.AllowedEmailDomains, diags.d)
	if !sameEmailDomainSet(configured, stored) {
		if len(stored) == 0 {
			m.AllowedEmailDomains = types.SetNull(types.StringType)
		} else {
			m.AllowedEmailDomains = setFromStringSlice(ctx, stored, diags.d)
		}
	}
	// The stored stylesheet is the sanitized, scoped form of what was sent, so
	// it never equals the config text; keep the config unless the page has none.
	if deref(resp.CustomCSS) == "" {
		m.CustomCSS = types.StringNull()
	} else {
		m.CustomCSS = keepOrTakeString(m.CustomCSS, resp.CustomCSS)
	}
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
}

// deref is nil-safe *string -> string.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *statusPageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toUpdateBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var created statusPageResponse
	path := r.client.Tenantf("/status", "/pages")
	if err := r.client.Post(ctx, path, plan.toCreateBody(), &created); err != nil {
		resp.Diagnostics.AddError("Error creating status page", err.Error())
		return
	}

	// CreatePageRequest carries only slug/name/description/dashboard, so every
	// other attribute is applied by a follow-up PATCH. It runs unconditionally:
	// the audience gate rejects PasswordProtected without a password (and
	// EmailDomainRestricted without domains), so both halves have to travel in
	// the same request.
	var updated statusPageResponse
	patchPath := r.client.Tenantf("/status", "/pages/%s", created.ID)
	if err := r.client.Patch(ctx, patchPath, body, &updated); err != nil {
		resp.Diagnostics.AddError("Error creating status page", err.Error())
		return
	}
	plan.applyResponse(ctx, &updated, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got statusPageResponse
	getPath := r.client.Tenantf("/status", "/pages/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading status page", err.Error())
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyResponse(ctx, &got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toUpdateBody(ctx, ds)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated statusPageResponse
	path := r.client.Tenantf("/status", "/pages/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, &updated); err != nil {
		resp.Diagnostics.AddError("Error updating status page", err.Error())
		return
	}

	plan.applyResponse(ctx, &updated, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/status", "/pages/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting status page", err.Error())
	}
}

func (r *statusPageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
