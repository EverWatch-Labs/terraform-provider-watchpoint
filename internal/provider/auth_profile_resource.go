package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An auth profile stores reusable, encrypted upstream credentials that monitors
// reference via `{{auth.NAME.<field>}}` interpolation (for example
// `{{auth.PaymentsApi.access_token}}`). Each profile has a single `kind` and a
// flat set of fields; only the fields relevant to that kind are honoured by the
// backend. Secret fields (client secret, password, refresh token, static key,
// client cert/key) are write-only: the provider sends them on create/update but
// the API never returns them (the response is masked with `has*` booleans), so
// the configured values are kept in state from config and out-of-band rotation
// is not drift-detected, exactly like watchpoint_vault_secret.value.
//
// API MAPPING (single source of truth): the wire DTOs, toUpsertBody, and
// applyResponse below are the ONE place the provider couples to the Monitoring.Api
// AuthProfile contract. The contract is FLAT (ASP.NET camelCase JSON):
//
//   POST   /monitoring/tenants/{tenant}/auth-profiles            -> authProfileResponse (with id)
//   GET    /monitoring/tenants/{tenant}/auth-profiles/{id}       -> authProfileResponse
//   PUT    /monitoring/tenants/{tenant}/auth-profiles/{id}       -> (re-GET)
//   DELETE /monitoring/tenants/{tenant}/auth-profiles/{id}
//
// The response is masked: secrets are never echoed back. Instead the API returns
// has* booleans (hasClientSecret, hasPassword, ...). To keep an existing secret on
// update, omit the field (or send the masked sentinel "********"); since Terraform
// always sends the configured value, the provider just sends the configured secret
// on every apply and never reads secrets back from the response.

var (
	_ resource.Resource                = (*authProfileResource)(nil)
	_ resource.ResourceWithConfigure   = (*authProfileResource)(nil)
	_ resource.ResourceWithImportState = (*authProfileResource)(nil)
)

// NewAuthProfileResource is the resource constructor wired into the provider.
func NewAuthProfileResource() resource.Resource { return &authProfileResource{} }

type authProfileResource struct {
	client *client.Client
}

// authProfileModel maps schema attributes <-> Terraform state. All fields are
// flat; only the ones relevant to `kind` are used by the backend.
type authProfileModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Kind        types.String `tfsdk:"kind"`
	Description types.String `tfsdk:"description"`

	// OAuth2* (all OAuth2 kinds).
	TokenEndpoint    types.String `tfsdk:"token_endpoint"`
	ClientID         types.String `tfsdk:"client_id"`
	ClientSecret     types.String `tfsdk:"client_secret"`
	Scopes           types.String `tfsdk:"scopes"`
	Audience         types.String `tfsdk:"audience"`
	ClientAuthMethod types.String `tfsdk:"client_auth_method"`

	// OAuth2Password.
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`

	// OAuth2RefreshToken.
	RefreshToken types.String `tfsdk:"refresh_token"`

	// LoginFlow.
	LoginFlowJSON types.String `tfsdk:"login_flow_json"`

	// StaticApiKey.
	StaticKey      types.String `tfsdk:"static_key"`
	VaultSecretRef types.String `tfsdk:"vault_secret_ref"`

	// Mtls.
	ClientCertPEM types.String `tfsdk:"client_cert_pem"`
	ClientKeyPEM  types.String `tfsdk:"client_key_pem"`

	// Token shaping (all kinds except Mtls).
	TokenPrefix        types.String `tfsdk:"token_prefix"`
	RefreshSkewSeconds types.Int64  `tfsdk:"refresh_skew_seconds"`
	DefaultTTLSeconds  types.Int64  `tfsdk:"default_ttl_seconds"`

	// Read-only / masked indicators.
	HasClientSecret types.Bool   `tfsdk:"has_client_secret"`
	HasPassword     types.Bool   `tfsdk:"has_password"`
	HasRefreshToken types.Bool   `tfsdk:"has_refresh_token"`
	HasStaticKey    types.Bool   `tfsdk:"has_static_key"`
	HasClientCert   types.Bool   `tfsdk:"has_client_cert"`
	HasClientKey    types.Bool   `tfsdk:"has_client_key"`
	LastUsedAt      types.String `tfsdk:"last_used_at"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

// --- Wire DTOs (flat camelCase JSON matching the monitoring API) ---
//
// Secret fields use pointers and omitempty so an update can omit a secret the
// practitioner did not re-supply; in practice Terraform always re-sends the
// configured value, so the secret is sent on every apply.

type authProfileUpsertBody struct {
	Name        *string `json:"name,omitempty"`
	Kind        *string `json:"kind,omitempty"`
	Description *string `json:"description,omitempty"`

	TokenEndpoint    *string `json:"tokenEndpoint,omitempty"`
	ClientID         *string `json:"clientId,omitempty"`
	ClientSecret     *string `json:"clientSecret,omitempty"`
	Scopes           *string `json:"scopes,omitempty"`
	Audience         *string `json:"audience,omitempty"`
	ClientAuthMethod *string `json:"clientAuthMethod,omitempty"`

	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`

	RefreshToken *string `json:"refreshToken,omitempty"`

	LoginFlowJSON *string `json:"loginFlowJson,omitempty"`

	StaticKey      *string `json:"staticKey,omitempty"`
	VaultSecretRef *string `json:"vaultSecretRef,omitempty"`

	ClientCertPEM *string `json:"clientCertPem,omitempty"`
	ClientKeyPEM  *string `json:"clientKeyPem,omitempty"`

	TokenPrefix        *string `json:"tokenPrefix,omitempty"`
	RefreshSkewSeconds *int64  `json:"refreshSkewSeconds,omitempty"`
	DefaultTTLSeconds  *int64  `json:"defaultTtlSeconds,omitempty"`
}

// authProfileResponse mirrors the masked GET/POST/PUT payload. Secrets are never
// returned; instead has* booleans report whether a secret is stored.
type authProfileResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Description *string `json:"description"`

	TokenEndpoint    *string `json:"tokenEndpoint"`
	ClientID         *string `json:"clientId"`
	Scopes           *string `json:"scopes"`
	Audience         *string `json:"audience"`
	ClientAuthMethod *string `json:"clientAuthMethod"`

	Username *string `json:"username"`

	LoginFlowJSON  *string `json:"loginFlowJson"`
	VaultSecretRef *string `json:"vaultSecretRef"`

	TokenPrefix        *string `json:"tokenPrefix"`
	RefreshSkewSeconds *int64  `json:"refreshSkewSeconds"`
	DefaultTTLSeconds  *int64  `json:"defaultTtlSeconds"`

	HasClientSecret bool `json:"hasClientSecret"`
	HasPassword     bool `json:"hasPassword"`
	HasRefreshToken bool `json:"hasRefreshToken"`
	HasStaticKey    bool `json:"hasStaticKey"`
	HasClientCert   bool `json:"hasClientCert"`
	HasClientKey    bool `json:"hasClientKey"`

	LastUsedAt *string `json:"lastUsedAt"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  *string `json:"updatedAt"`
}

func (r *authProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_profile"
}

func (r *authProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *authProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A reusable, encrypted upstream-credential profile referenced by monitors via " +
			"`{{auth.NAME.<field>}}` interpolation (for example `{{auth.PaymentsApi.access_token}}`). " +
			"Each profile has a single `kind`; only the fields relevant to that kind are used. Secret " +
			"fields are write-only: they are sent on create/update but never read back (the API response " +
			"is masked), so out-of-band rotation is not drift-detected.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Auth profile ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Unique name within the tenant, used in `{{auth.NAME.<field>}}` interpolation. " +
					"Must match `^[A-Za-z][A-Za-z0-9_]{0,127}$`. Immutable: changing it forces replacement.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"kind": schema.StringAttribute{
				MarkdownDescription: "Credential kind. One of `OAuth2ClientCredentials`, `OAuth2Password`, " +
					"`OAuth2RefreshToken`, `LoginFlow`, `StaticApiKey`, `Mtls`. Immutable: changing it forces " +
					"replacement.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Human-readable description of what this credential is for.",
				Optional:            true,
			},

			// --- OAuth2* (all OAuth2 kinds) ---
			"token_endpoint": schema.StringAttribute{
				MarkdownDescription: "OAuth 2.0 token endpoint that issues the access token. Required for the " +
					"`OAuth2*` kinds.",
				Optional: true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "OAuth 2.0 client identifier.",
				Optional:            true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "OAuth 2.0 client secret. Write-only: sent on every apply but never read " +
					"back from the masked API response.",
				Optional:  true,
				Sensitive: true,
			},
			"scopes": schema.StringAttribute{
				MarkdownDescription: "Space-separated scopes requested with the token.",
				Optional:            true,
			},
			"audience": schema.StringAttribute{
				MarkdownDescription: "Optional audience parameter (used by some providers, e.g. Auth0).",
				Optional:            true,
			},
			"client_auth_method": schema.StringAttribute{
				MarkdownDescription: "How client credentials are presented to the token endpoint: `body` " +
					"(form parameters) or `basic` (HTTP Basic auth). Defaults to `body`.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("body"),
			},

			// --- OAuth2Password ---
			"username": schema.StringAttribute{
				MarkdownDescription: "Resource-owner username (`OAuth2Password`).",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Resource-owner password (`OAuth2Password`). Write-only: sent on every " +
					"apply but never read back.",
				Optional:  true,
				Sensitive: true,
			},

			// --- OAuth2RefreshToken ---
			"refresh_token": schema.StringAttribute{
				MarkdownDescription: "Long-lived refresh token (`OAuth2RefreshToken`). Write-only: sent on " +
					"every apply but never read back.",
				Optional:  true,
				Sensitive: true,
			},

			// --- LoginFlow ---
			"login_flow_json": schema.StringAttribute{
				MarkdownDescription: "JSON document describing the custom login flow (`LoginFlow`). Sent as-is.",
				Optional:            true,
			},

			// --- StaticApiKey ---
			"static_key": schema.StringAttribute{
				MarkdownDescription: "The static API key value (`StaticApiKey`). Write-only: sent on every " +
					"apply but never read back. Mutually exclusive with `vault_secret_ref`.",
				Optional:  true,
				Sensitive: true,
			},
			"vault_secret_ref": schema.StringAttribute{
				MarkdownDescription: "Name of a vault secret (without `{{ }}`) holding the static API key " +
					"(`StaticApiKey`). Mutually exclusive with `static_key`.",
				Optional: true,
			},

			// --- Mtls ---
			"client_cert_pem": schema.StringAttribute{
				MarkdownDescription: "Client certificate in PEM format (`Mtls`). Write-only: sent on every " +
					"apply but never read back.",
				Optional:  true,
				Sensitive: true,
			},
			"client_key_pem": schema.StringAttribute{
				MarkdownDescription: "Client private key in PEM format (`Mtls`). Write-only: sent on every " +
					"apply but never read back.",
				Optional:  true,
				Sensitive: true,
			},

			// --- Token shaping (all kinds except Mtls) ---
			"token_prefix": schema.StringAttribute{
				MarkdownDescription: "Prefix prepended to the token when injected into requests. Defaults to " +
					"`Bearer ` (note the trailing space).",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("Bearer "),
			},
			"refresh_skew_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds before expiry at which the token is proactively refreshed. " +
					"Defaults to `60`.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(60),
			},
			"default_ttl_seconds": schema.Int64Attribute{
				MarkdownDescription: "Fallback token lifetime in seconds when the provider does not return " +
					"one. Defaults to `3600`.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(3600),
			},

			// --- Read-only / masked indicators ---
			"has_client_secret": schema.BoolAttribute{
				MarkdownDescription: "Whether a client secret is stored.",
				Computed:            true,
			},
			"has_password": schema.BoolAttribute{
				MarkdownDescription: "Whether a password is stored.",
				Computed:            true,
			},
			"has_refresh_token": schema.BoolAttribute{
				MarkdownDescription: "Whether a refresh token is stored.",
				Computed:            true,
			},
			"has_static_key": schema.BoolAttribute{
				MarkdownDescription: "Whether a static API key is stored.",
				Computed:            true,
			},
			"has_client_cert": schema.BoolAttribute{
				MarkdownDescription: "Whether a client certificate is stored.",
				Computed:            true,
			},
			"has_client_key": schema.BoolAttribute{
				MarkdownDescription: "Whether a client private key is stored.",
				Computed:            true,
			},
			"last_used_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp the credential was last used (RFC 3339), if ever.",
				Computed:            true,
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

// toUpsertBody serializes the model onto the flat wire DTO. Unset optional fields
// are omitted (nil) so the backend ignores fields irrelevant to the kind.
func (m *authProfileModel) toUpsertBody() *authProfileUpsertBody {
	return &authProfileUpsertBody{
		Name:        strPtr(m.Name),
		Kind:        strPtr(m.Kind),
		Description: strPtr(m.Description),

		TokenEndpoint:    strPtr(m.TokenEndpoint),
		ClientID:         strPtr(m.ClientID),
		ClientSecret:     strPtr(m.ClientSecret),
		Scopes:           strPtr(m.Scopes),
		Audience:         strPtr(m.Audience),
		ClientAuthMethod: strPtr(m.ClientAuthMethod),

		Username: strPtr(m.Username),
		Password: strPtr(m.Password),

		RefreshToken: strPtr(m.RefreshToken),

		LoginFlowJSON: strPtr(m.LoginFlowJSON),

		StaticKey:      strPtr(m.StaticKey),
		VaultSecretRef: strPtr(m.VaultSecretRef),

		ClientCertPEM: strPtr(m.ClientCertPEM),
		ClientKeyPEM:  strPtr(m.ClientKeyPEM),

		TokenPrefix:        strPtr(m.TokenPrefix),
		RefreshSkewSeconds: int64Ptr(m.RefreshSkewSeconds),
		DefaultTTLSeconds:  int64Ptr(m.DefaultTTLSeconds),
	}
}

// applyResponse copies the masked API response into the model. Write-only secrets
// (client_secret, password, refresh_token, static_key, client_cert_pem,
// client_key_pem) are deliberately NOT touched: the model keeps whatever the
// practitioner configured, mirroring watchpoint_vault_secret.value. Non-secret
// fields are refreshed in place; nulls in the response do not clobber configured
// values (applyStrIfPresent), which keeps server-defaulted optionals stable.
func (m *authProfileModel) applyResponse(resp *authProfileResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Kind = types.StringValue(resp.Kind)
	m.Description = strFromPtr(resp.Description)

	applyStrIfPresent(&m.TokenEndpoint, resp.TokenEndpoint)
	applyStrIfPresent(&m.ClientID, resp.ClientID)
	applyStrIfPresent(&m.Scopes, resp.Scopes)
	applyStrIfPresent(&m.Audience, resp.Audience)
	applyStrIfPresent(&m.ClientAuthMethod, resp.ClientAuthMethod)
	applyStrIfPresent(&m.Username, resp.Username)
	applyStrIfPresent(&m.LoginFlowJSON, resp.LoginFlowJSON)
	applyStrIfPresent(&m.VaultSecretRef, resp.VaultSecretRef)
	applyStrIfPresent(&m.TokenPrefix, resp.TokenPrefix)

	if resp.RefreshSkewSeconds != nil {
		m.RefreshSkewSeconds = types.Int64Value(*resp.RefreshSkewSeconds)
	}
	if resp.DefaultTTLSeconds != nil {
		m.DefaultTTLSeconds = types.Int64Value(*resp.DefaultTTLSeconds)
	}

	m.HasClientSecret = types.BoolValue(resp.HasClientSecret)
	m.HasPassword = types.BoolValue(resp.HasPassword)
	m.HasRefreshToken = types.BoolValue(resp.HasRefreshToken)
	m.HasStaticKey = types.BoolValue(resp.HasStaticKey)
	m.HasClientCert = types.BoolValue(resp.HasClientCert)
	m.HasClientKey = types.BoolValue(resp.HasClientKey)

	m.LastUsedAt = strFromPtr(resp.LastUsedAt)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = strFromPtr(resp.UpdatedAt)
}

// applyStrIfPresent refreshes a model string only when the API actually returned
// a value, so a null in the response does not clobber a configured (and possibly
// server-defaulted) value into null and churn the plan.
func applyStrIfPresent(dst *types.String, src *string) {
	if src != nil {
		*dst = types.StringValue(*src)
	}
}

func (r *authProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan authProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create returns the created (masked) object directly. Configured secrets are
	// left untouched by applyResponse.
	var got authProfileResponse
	path := r.client.Tenantf("/monitoring", "/auth-profiles")
	if err := r.client.Post(ctx, path, plan.toUpsertBody(), &got); err != nil {
		resp.Diagnostics.AddError("Error creating auth profile", err.Error())
		return
	}

	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *authProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state authProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got authProfileResponse
	getPath := r.client.Tenantf("/monitoring", "/auth-profiles/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading auth profile", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *authProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan authProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/auth-profiles/%s", plan.ID.ValueString())
	if err := r.client.Put(ctx, path, plan.toUpsertBody(), nil); err != nil {
		resp.Diagnostics.AddError("Error updating auth profile", err.Error())
		return
	}

	var got authProfileResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading auth profile after update", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *authProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state authProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/auth-profiles/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting auth profile", err.Error())
	}
}

func (r *authProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
