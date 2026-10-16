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

// A tenant SSO configuration (SsoController). providerType is sent to the API as
// a PascalCase enum name (e.g. "EntraId", "Okta", "OidcGeneric"); the API also
// accepts an int. The response returns it as a string ("providerType") plus an
// int ("providerTypeValue"); we model it as a string. Create returns only
// {id,success,callbackUrl}, so Create re-reads via GET-by-id. The write-only
// secrets oidcClientSecret and samlCertificate are never refreshed on Read.

var (
	_ resource.Resource                = (*ssoConfigurationResource)(nil)
	_ resource.ResourceWithConfigure   = (*ssoConfigurationResource)(nil)
	_ resource.ResourceWithImportState = (*ssoConfigurationResource)(nil)
)

// NewSSOConfigurationResource is the resource constructor wired into the provider.
func NewSSOConfigurationResource() resource.Resource { return &ssoConfigurationResource{} }

type ssoConfigurationResource struct {
	client *client.Client
}

// ssoConfigurationModel maps schema attributes <-> Terraform state.
type ssoConfigurationModel struct {
	ID                     types.String `tfsdk:"id"`
	ConnectionName         types.String `tfsdk:"connection_name"`
	ProviderType           types.String `tfsdk:"provider_type"`
	Domain                 types.String `tfsdk:"domain"`
	ClientID               types.String `tfsdk:"client_id"`
	SamlEntityID           types.String `tfsdk:"saml_entity_id"`
	SamlSsoURL             types.String `tfsdk:"saml_sso_url"`
	SamlCertificate        types.String `tfsdk:"saml_certificate"`
	SamlSigningAlgorithm   types.String `tfsdk:"saml_signing_algorithm"`
	OidcAuthority          types.String `tfsdk:"oidc_authority"`
	OidcClientSecret       types.String `tfsdk:"oidc_client_secret"`
	OidcScopes             types.String `tfsdk:"oidc_scopes"`
	GroupClaimName         types.String `tfsdk:"group_claim_name"`
	JitProvisioningEnabled types.Bool   `tfsdk:"jit_provisioning_enabled"`
	EnforceSsoOnly         types.Bool   `tfsdk:"enforce_sso_only"`
	DefaultRoleID          types.String `tfsdk:"default_role_id"`
	IsEnabled              types.Bool   `tfsdk:"is_enabled"`
	CreatedAt              types.String `tfsdk:"created_at"`
	UpdatedAt              types.String `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the Identity API) ---

type ssoConfigurationUpsertBody struct {
	ConnectionName         *string `json:"connectionName,omitempty"`
	ProviderType           *string `json:"providerType,omitempty"`
	Domain                 *string `json:"domain,omitempty"`
	ClientID               *string `json:"clientId,omitempty"`
	SamlEntityID           *string `json:"samlEntityId,omitempty"`
	SamlSsoURL             *string `json:"samlSsoUrl,omitempty"`
	SamlCertificate        *string `json:"samlCertificate,omitempty"`
	SamlSigningAlgorithm   *string `json:"samlSigningAlgorithm,omitempty"`
	OidcAuthority          *string `json:"oidcAuthority,omitempty"`
	OidcClientSecret       *string `json:"oidcClientSecret,omitempty"`
	OidcScopes             *string `json:"oidcScopes,omitempty"`
	GroupClaimName         *string `json:"groupClaimName,omitempty"`
	JitProvisioningEnabled *bool   `json:"jitProvisioningEnabled,omitempty"`
	EnforceSsoOnly         *bool   `json:"enforceSsoOnly,omitempty"`
	DefaultRoleID          *string `json:"defaultRoleId,omitempty"`
	IsEnabled              *bool   `json:"isEnabled,omitempty"`
}

type ssoConfigurationResponse struct {
	ID                     string  `json:"id"`
	ConnectionName         string  `json:"connectionName"`
	ProviderType           string  `json:"providerType"`
	Domain                 *string `json:"domain"`
	ClientID               *string `json:"clientId"`
	SamlEntityID           *string `json:"samlEntityId"`
	SamlSsoURL             *string `json:"samlSsoUrl"`
	SamlSigningAlgorithm   *string `json:"samlSigningAlgorithm"`
	OidcAuthority          *string `json:"oidcAuthority"`
	OidcScopes             *string `json:"oidcScopes"`
	GroupClaimName         *string `json:"groupClaimName"`
	JitProvisioningEnabled bool    `json:"jitProvisioningEnabled"`
	EnforceSsoOnly         bool    `json:"enforceSsoOnly"`
	DefaultRoleID          *string `json:"defaultRoleId"`
	IsEnabled              bool    `json:"isEnabled"`
	CreatedAt              string  `json:"createdAt"`
	UpdatedAt              *string `json:"updatedAt"`
}

func (r *ssoConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sso_configuration"
}

func (r *ssoConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *ssoConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A tenant SSO configuration (SAML or OIDC). The client secret and SAML certificate " +
			"are write-only and are never read back from the API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "SSO configuration ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"connection_name": schema.StringAttribute{
				MarkdownDescription: "Display name for the connection.",
				Optional:            true,
				Computed:            true,
			},
			"provider_type": schema.StringAttribute{
				MarkdownDescription: "SSO provider type. One of `EntraId`, `Okta`, `GoogleWorkspace`, " +
					"`SamlGeneric`, `OidcGeneric`, `PingFederate`, `Adfs`.",
				Required: true,
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "Email domain associated with the connection. Must be a verified domain.",
				Optional:            true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "OIDC client ID.",
				Optional:            true,
			},
			"saml_entity_id": schema.StringAttribute{
				MarkdownDescription: "SAML entity (issuer) ID.",
				Optional:            true,
			},
			"saml_sso_url": schema.StringAttribute{
				MarkdownDescription: "SAML SSO (sign-in) URL.",
				Optional:            true,
			},
			"saml_certificate": schema.StringAttribute{
				MarkdownDescription: "SAML signing certificate (PEM). Write-only: not refreshed on read.",
				Optional:            true,
				Sensitive:           true,
			},
			"saml_signing_algorithm": schema.StringAttribute{
				MarkdownDescription: "SAML signing algorithm.",
				Optional:            true,
			},
			"oidc_authority": schema.StringAttribute{
				MarkdownDescription: "OIDC authority / issuer URL.",
				Optional:            true,
			},
			"oidc_client_secret": schema.StringAttribute{
				MarkdownDescription: "OIDC client secret. Write-only: not refreshed on read.",
				Optional:            true,
				Sensitive:           true,
			},
			"oidc_scopes": schema.StringAttribute{
				MarkdownDescription: "Space- or comma-separated OIDC scopes. Defaults to `openid profile email`.",
				Optional:            true,
				Computed:            true,
			},
			"group_claim_name": schema.StringAttribute{
				MarkdownDescription: "Name of the claim carrying group memberships. Defaults to `groups`.",
				Optional:            true,
				Computed:            true,
			},
			"jit_provisioning_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether just-in-time user provisioning is enabled. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
			},
			"enforce_sso_only": schema.BoolAttribute{
				MarkdownDescription: "Whether to require SSO for sign-in. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
			},
			"default_role_id": schema.StringAttribute{
				MarkdownDescription: "Role assigned to JIT-provisioned users by default.",
				Optional:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the connection is enabled. Defaults to `true`.",
				Optional:            true,
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

func (m *ssoConfigurationModel) toUpsertBody() *ssoConfigurationUpsertBody {
	return &ssoConfigurationUpsertBody{
		ConnectionName:         strPtr(m.ConnectionName),
		ProviderType:           strPtr(m.ProviderType),
		Domain:                 strPtr(m.Domain),
		ClientID:               strPtr(m.ClientID),
		SamlEntityID:           strPtr(m.SamlEntityID),
		SamlSsoURL:             strPtr(m.SamlSsoURL),
		SamlCertificate:        strPtr(m.SamlCertificate),
		SamlSigningAlgorithm:   strPtr(m.SamlSigningAlgorithm),
		OidcAuthority:          strPtr(m.OidcAuthority),
		OidcClientSecret:       strPtr(m.OidcClientSecret),
		OidcScopes:             strPtr(m.OidcScopes),
		GroupClaimName:         strPtr(m.GroupClaimName),
		JitProvisioningEnabled: boolPtr(m.JitProvisioningEnabled),
		EnforceSsoOnly:         boolPtr(m.EnforceSsoOnly),
		DefaultRoleID:          strPtr(m.DefaultRoleID),
		IsEnabled:              boolPtr(m.IsEnabled),
	}
}

// applyResponse copies API response fields into the model. The write-only
// secrets (oidc_client_secret, saml_certificate) are intentionally left
// untouched so configured values survive reads.
func (m *ssoConfigurationModel) applyResponse(resp *ssoConfigurationResponse) {
	m.ID = types.StringValue(resp.ID)
	m.ConnectionName = types.StringValue(resp.ConnectionName)
	m.ProviderType = types.StringValue(resp.ProviderType)
	m.Domain = strFromPtr(resp.Domain)
	m.ClientID = strFromPtr(resp.ClientID)
	m.SamlEntityID = strFromPtr(resp.SamlEntityID)
	m.SamlSsoURL = strFromPtr(resp.SamlSsoURL)
	m.SamlSigningAlgorithm = strFromPtr(resp.SamlSigningAlgorithm)
	m.OidcAuthority = strFromPtr(resp.OidcAuthority)
	m.OidcScopes = strFromPtr(resp.OidcScopes)
	m.GroupClaimName = strFromPtr(resp.GroupClaimName)
	m.JitProvisioningEnabled = types.BoolValue(resp.JitProvisioningEnabled)
	m.EnforceSsoOnly = types.BoolValue(resp.EnforceSsoOnly)
	m.DefaultRoleID = strFromPtr(resp.DefaultRoleID)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = strFromPtr(resp.UpdatedAt)
}

func (r *ssoConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ssoConfigurationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created idNameResponse
	path := r.client.Tenantf("/identity", "/sso")
	if err := r.client.Post(ctx, path, plan.toUpsertBody(), &created); err != nil {
		resp.Diagnostics.AddError("Error creating SSO configuration", err.Error())
		return
	}

	var got ssoConfigurationResponse
	getPath := r.client.Tenantf("/identity", "/sso/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading SSO configuration after create", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ssoConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ssoConfigurationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got ssoConfigurationResponse
	getPath := r.client.Tenantf("/identity", "/sso/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading SSO configuration", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ssoConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ssoConfigurationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/identity", "/sso/%s", plan.ID.ValueString())
	if err := r.client.Put(ctx, path, plan.toUpsertBody(), nil); err != nil {
		resp.Diagnostics.AddError("Error updating SSO configuration", err.Error())
		return
	}

	var got ssoConfigurationResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading SSO configuration after update", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ssoConfigurationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ssoConfigurationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/identity", "/sso/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting SSO configuration", err.Error())
	}
}

func (r *ssoConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
