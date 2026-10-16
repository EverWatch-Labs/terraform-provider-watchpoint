package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An external vault connection points Watchpoint at a vault you already run, so a
// vault secret can be a REFERENCE into it rather than ciphertext we hold.
//
// NOTHING SECRET LIVES IN THIS RESOURCE. It carries endpoints, directory and client
// ids, and at most the NAME of a credential kept elsewhere. That is deliberate and it
// is why the whole thing is safe in a Terraform state file: for every token_source
// except `client_credentials` there is no credential anywhere, and for that one the
// state holds a name, not a value.
//
// API MAPPING (single source of truth for this resource):
//
//	GET    /monitoring/tenants/{tenant}/vault/connections            -> []externalVaultResponse
//	PUT    /monitoring/tenants/{tenant}/vault/connections            -> externalVaultResponse (upsert by slug)
//	DELETE /monitoring/tenants/{tenant}/vault/connections/{slug}
//
// There is no GET-by-slug, so Read filters the list. Upsert is a PUT with the slug in
// the BODY rather than the path, which is why Create and Update share one call.
var (
	_ resource.Resource                = (*externalVaultResource)(nil)
	_ resource.ResourceWithConfigure   = (*externalVaultResource)(nil)
	_ resource.ResourceWithImportState = (*externalVaultResource)(nil)
)

// NewExternalVaultResource is the resource constructor wired into the provider.
func NewExternalVaultResource() resource.Resource { return &externalVaultResource{} }

type externalVaultResource struct {
	client *client.Client
}

type externalVaultModel struct {
	ID                  types.String `tfsdk:"id"`
	Slug                types.String `tfsdk:"slug"`
	Name                types.String `tfsdk:"name"`
	Kind                types.String `tfsdk:"kind"`
	TokenSource         types.String `tfsdk:"token_source"`
	Resolution          types.String `tfsdk:"resolution"`
	Config              types.Map    `tfsdk:"config"`
	CredentialSecretKey types.String `tfsdk:"credential_secret_key"`
	AllowBrowse         types.Bool   `tfsdk:"allow_browse"`

	LastCheckedAt types.String `tfsdk:"last_checked_at"`
	LastCheckOK   types.Bool   `tfsdk:"last_check_ok"`
	LastError     types.String `tfsdk:"last_error"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

type externalVaultUpsertBody struct {
	Slug                string            `json:"slug"`
	Name                string            `json:"name"`
	Kind                string            `json:"kind"`
	TokenSource         string            `json:"tokenSource"`
	Resolution          *string           `json:"resolution,omitempty"`
	Config              map[string]string `json:"config,omitempty"`
	CredentialSecretKey *string           `json:"credentialSecretKey,omitempty"`
	AllowBrowse         bool              `json:"allowBrowse"`
}

type externalVaultResponse struct {
	ID                  string            `json:"id"`
	Slug                string            `json:"slug"`
	Name                string            `json:"name"`
	Kind                string            `json:"kind"`
	TokenSource         string            `json:"tokenSource"`
	Resolution          string            `json:"resolution"`
	Config              map[string]string `json:"config"`
	CredentialSecretKey string            `json:"credentialSecretKey"`
	AllowBrowse         bool              `json:"allowBrowse"`

	LastCheckedAt *string `json:"lastCheckedAt"`
	LastCheckOK   bool    `json:"lastCheckOk"`
	LastError     string  `json:"lastError"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     *string `json:"updatedAt"`
}

func (r *externalVaultResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_external_vault"
}

func (r *externalVaultResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *externalVaultResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A link to a vault you already run, so a `watchpoint_vault_secret` can reference a " +
			"secret in it instead of storing a copy here. Nothing secret is stored in this resource: it holds " +
			"endpoints, directory and client ids, and at most the NAME of a credential kept elsewhere.\n\n" +
			"Use the `watchpoint_external_vault_federation` data source to build the trust policy on the vendor " +
			"side from the same values Watchpoint will actually present.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Connection ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: "Stable identifier every stored secret reference points at. " +
					"Immutable: changing it would leave existing references pointing at a connection that no longer exists.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name.",
				Required:            true,
			},
			"kind": schema.StringAttribute{
				MarkdownDescription: "Vendor: `azure-key-vault`, `aws-secrets-manager`, `gcp-secret-manager`, " +
					"`hashicorp-vault`, `cyberark-conjur`, `cyberark-ccp`, `delinea-secret-server`, " +
					"`beyondtrust-password-safe` or `akeyless`. Immutable: the API refuses to repoint a slug at a " +
					"different vendor, because every stored reference carries vendor-shaped details that would be " +
					"left intact and wrong.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"token_source": schema.StringAttribute{
				MarkdownDescription: "How Watchpoint authenticates:\n\n" +
					"  * `federated` - nothing stored. Watchpoint presents a short-lived assertion and you " +
					"configure (and can revoke) the trust on your side. Prefer this wherever the vendor offers it.\n" +
					"  * `client_credentials` - the one mode that stores a credential. The credential lives in a " +
					"`watchpoint_vault_secret` and is named here by `credential_secret_key`, never inlined.\n" +
					"  * `mtls` / `local` - the fetch happens on a host inside your network; the certificate or " +
					"vendor agent never reaches Watchpoint.\n\n" +
					"Not every vendor supports every mode; the API rejects an unsupported combination on apply.",
				Required: true,
			},
			"resolution": schema.StringAttribute{
				MarkdownDescription: "`control-plane` (default) fetches from Watchpoint. `agent` hands out the " +
					"reference instead so the secret is fetched inside your network and never transits Watchpoint " +
					"at all. Required for vaults that are not routable from the internet, and stronger anyway.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("control-plane"),
			},
			"config": schema.MapAttribute{
				MarkdownDescription: "Per-vendor non-secret settings, for example `vault_uri` for Azure, " +
					"`region` and `role_arn` for AWS, `address` and `mount` for HashiCorp Vault. The exact keys a " +
					"vendor needs are listed on the External Vaults screen in the portal. Nothing secret belongs here.",
				ElementType: types.StringType,
				Optional:    true,
			},
			"credential_secret_key": schema.StringAttribute{
				MarkdownDescription: "The KEY of a `watchpoint_vault_secret` holding the credential. Only valid " +
					"with `token_source = \"client_credentials\"`, and required there; every other mode must leave " +
					"it empty. This is a name, not a value, so it is safe in state.",
				Optional: true,
			},
			"allow_browse": schema.BoolAttribute{
				MarkdownDescription: "Allow listing secret NAMES in this vault, for the portal's picker. Off by " +
					"default: enumerating needs a wider grant on the vendor's side than fetching does, and a secret " +
					"name is itself disclosure. Only accepted for vendors whose adapter implements listing.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"last_checked_at": schema.StringAttribute{
				MarkdownDescription: "When this connection was last tested (RFC 3339). Cleared on every save, " +
					"because a green tick measured against a configuration that no longer exists is worse than none.",
				Computed: true,
			},
			"last_check_ok": schema.BoolAttribute{
				MarkdownDescription: "Whether the last test authenticated successfully.",
				Computed:            true,
			},
			"last_error": schema.StringAttribute{
				MarkdownDescription: "The vendor's own message from the last failed test.",
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

func (m *externalVaultModel) toUpsertBody(ctx context.Context, diags *diagSink) *externalVaultUpsertBody {
	cfg := map[string]string{}
	if !m.Config.IsNull() && !m.Config.IsUnknown() {
		diags.d.Append(m.Config.ElementsAs(ctx, &cfg, false)...)
	}

	return &externalVaultUpsertBody{
		Slug:                m.Slug.ValueString(),
		Name:                m.Name.ValueString(),
		Kind:                m.Kind.ValueString(),
		TokenSource:         m.TokenSource.ValueString(),
		Resolution:          strPtr(m.Resolution),
		Config:              cfg,
		CredentialSecretKey: strPtr(m.CredentialSecretKey),
		AllowBrowse:         m.AllowBrowse.ValueBool(),
	}
}

func (m *externalVaultModel) applyResponse(ctx context.Context, resp *externalVaultResponse, diags *diagSink) {
	m.ID = types.StringValue(resp.ID)
	m.Slug = types.StringValue(resp.Slug)
	m.Name = types.StringValue(resp.Name)
	m.Kind = types.StringValue(resp.Kind)
	m.TokenSource = types.StringValue(resp.TokenSource)
	m.Resolution = types.StringValue(resp.Resolution)
	m.AllowBrowse = types.BoolValue(resp.AllowBrowse)

	// The API normalises an unset credential to "". Mapping that back to a null keeps
	// a config that simply omits the attribute from showing a permanent diff.
	if resp.CredentialSecretKey == "" {
		m.CredentialSecretKey = types.StringNull()
	} else {
		m.CredentialSecretKey = types.StringValue(resp.CredentialSecretKey)
	}

	// Same reasoning for config: an omitted map round-trips as an empty object, and
	// writing that back as an empty map would diff against a null one forever.
	if len(resp.Config) == 0 {
		m.Config = types.MapNull(types.StringType)
	} else {
		v, d := types.MapValueFrom(ctx, types.StringType, resp.Config)
		diags.d.Append(d...)
		m.Config = v
	}

	m.LastCheckedAt = strFromPtr(resp.LastCheckedAt)
	m.LastCheckOK = types.BoolValue(resp.LastCheckOK)
	m.LastError = types.StringValue(resp.LastError)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = strFromPtr(resp.UpdatedAt)
}

// upsert is shared by Create and Update: the API addresses a connection by the slug in
// the body, so both are the same PUT.
func (r *externalVaultResource) upsert(ctx context.Context, plan *externalVaultModel, diags *diagSink) bool {
	body := plan.toUpsertBody(ctx, diags)
	if diags.d.HasError() {
		return false
	}

	var got externalVaultResponse
	if err := r.client.Put(ctx, r.client.Tenantf("/monitoring", "/vault/connections"), body, &got); err != nil {
		diags.d.AddError("Error saving external vault connection", err.Error())
		return false
	}
	plan.applyResponse(ctx, &got, diags)
	return !diags.d.HasError()
}

func (r *externalVaultResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan externalVaultModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.upsert(ctx, &plan, &diagSink{&resp.Diagnostics}) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *externalVaultResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state externalVaultModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No GET-by-slug endpoint, so filter the list. It is small by nature: a tenant has
	// a handful of vaults, not thousands.
	var list []externalVaultResponse
	if err := r.client.Get(ctx, r.client.Tenantf("/monitoring", "/vault/connections"), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading external vault connections", err.Error())
		return
	}

	wanted := state.Slug.ValueString()
	for i := range list {
		if !strings.EqualFold(list[i].Slug, wanted) {
			continue
		}
		state.applyResponse(ctx, &list[i], &diagSink{&resp.Diagnostics})
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *externalVaultResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan externalVaultModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !r.upsert(ctx, &plan, &diagSink{&resp.Diagnostics}) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *externalVaultResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state externalVaultModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/vault/connections/%s", state.Slug.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		// A 409 here means secrets still resolve through this connection, and the API
		// names them. Pass that through: "in use" on its own leaves somebody running
		// destroy again.
		resp.Diagnostics.AddError(
			"Error deleting external vault connection",
			fmt.Sprintf("%s\n\nDeleting the connection does not lose the secrets, which stay in your vault. "+
				"It loses how Watchpoint reaches them, so every later resolve fails.", err.Error()),
		)
	}
}

// ImportState takes the SLUG, not the id: the slug is what the API addresses a
// connection by, what every stored reference points at, and what a practitioner
// actually has in front of them.
func (r *externalVaultResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("slug"), req, resp)
}
