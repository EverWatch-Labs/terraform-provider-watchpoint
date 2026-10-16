package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// vaultSecretResource manages a vault secret. The secret `value` is write-only:
// list/get-without-value responses never return it, and to keep state stable it
// is never clobbered on Read even when the single GET returns the decrypted value.
//
// A secret either holds a value Watchpoint encrypts, or `external_*` pointing at a
// secret in a vault you already run (see watchpoint_external_vault). Set one or the
// other, never both. Linking is the stronger option and it takes the value out of
// Terraform state entirely: there is nothing to encrypt because there is nothing here.
var (
	_ resource.Resource                     = (*vaultSecretResource)(nil)
	_ resource.ResourceWithConfigure        = (*vaultSecretResource)(nil)
	_ resource.ResourceWithImportState      = (*vaultSecretResource)(nil)
	_ resource.ResourceWithConfigValidators = (*vaultSecretResource)(nil)
)

// NewVaultSecretResource is the resource constructor wired into the provider.
func NewVaultSecretResource() resource.Resource { return &vaultSecretResource{} }

type vaultSecretResource struct {
	client *client.Client
}

// vaultSecretModel maps schema attributes <-> Terraform state.
type vaultSecretModel struct {
	ID          types.String `tfsdk:"id"`
	Key         types.String `tfsdk:"key"`
	Value       types.String `tfsdk:"value"`
	Description types.String `tfsdk:"description"`
	Category    types.String `tfsdk:"category"`
	HasValue    types.Bool   `tfsdk:"has_value"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`

	// Set these instead of `value` to point the key at a vault you run. Unlike
	// `value` they ARE drift-detected: the API returns the reference on every read
	// because a reference holds nothing secret.
	ExternalVault   types.String `tfsdk:"external_vault"`
	ExternalName    types.String `tfsdk:"external_name"`
	ExternalVersion types.String `tfsdk:"external_version"`
	ExternalField   types.String `tfsdk:"external_field"`
	Engine          types.String `tfsdk:"engine"`
}

// --- Wire DTOs (camelCase JSON matching the monitoring API) ---

// externalSecretRef is the pointer stored in place of a value. It is the same shape
// the platform stores and the vendor adapters read; a parallel shape here would be one
// more place for the field names to drift from what a fetch actually looks for.
type externalSecretRef struct {
	Connection string `json:"connection"`
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Field      string `json:"field,omitempty"`
}

type vaultSecretCreateBody struct {
	Key         string             `json:"key"`
	Value       *string            `json:"value,omitempty"`
	External    *externalSecretRef `json:"external,omitempty"`
	Description *string            `json:"description,omitempty"`
	Category    *string            `json:"category,omitempty"`
}

// vaultSecretUpdateBody is the PUT payload. Value is a pointer so it can be
// omitted to update only metadata.
type vaultSecretUpdateBody struct {
	Value       *string            `json:"value,omitempty"`
	External    *externalSecretRef `json:"external,omitempty"`
	Description *string            `json:"description,omitempty"`
	Category    *string            `json:"category,omitempty"`
}

// vaultSecretResponse mirrors MapSecret. value is null unless the single GET
// endpoint is used; we never copy it into state regardless.
type vaultSecretResponse struct {
	ID          string             `json:"id"`
	Key         string             `json:"key"`
	Description *string            `json:"description"`
	Category    *string            `json:"category"`
	HasValue    bool               `json:"hasValue"`
	Engine      string             `json:"engine"`
	External    *externalSecretRef `json:"external"`
	CreatedAt   string             `json:"createdAt"`
	UpdatedAt   *string            `json:"updatedAt"`
}

func (r *vaultSecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_secret"
}

func (r *vaultSecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *vaultSecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An encrypted vault secret referenced in monitors via `{{vault.KEY}}` interpolation. " +
			"The `value` is write-only: it is never read back from the API, so changing it out-of-band is not detected.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Vault secret ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "Unique key within the tenant, used in `{{vault.KEY}}` interpolation.",
				Required:            true,
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "Secret value. Write-only: the provider sends it on create/update but never " +
					"refreshes it on read, so it is not drift-detected. Omit on update to change only metadata. " +
					"Conflicts with `external_vault`; set one or the other.",
				Optional:  true,
				Sensitive: true,
			},
			"external_vault": schema.StringAttribute{
				MarkdownDescription: "The `slug` of a `watchpoint_external_vault` this secret is read from, " +
					"instead of storing a value here. Watchpoint keeps only the reference and fetches the value " +
					"when a check needs it, so nothing sensitive ends up in Terraform state either. " +
					"Requires `external_name`; conflicts with `value`.",
				Optional: true,
			},
			"external_name": schema.StringAttribute{
				MarkdownDescription: "Name of the secret inside that vault, in the vendor's own addressing: a " +
					"Key Vault secret name, an AWS secret name or ARN, a Vault path under the configured mount, " +
					"a Conjur variable id, or `<system id>/<account id>` for BeyondTrust Password Safe.",
				Optional: true,
			},
			"external_version": schema.StringAttribute{
				MarkdownDescription: "Pin a specific version. Leave unset to follow whatever is current, so your " +
					"own rotation takes effect with no Terraform change at all, which is usually what you want.",
				Optional: true,
			},
			"external_field": schema.StringAttribute{
				MarkdownDescription: "Selects one key out of a structured secret, so `{{vault.KEY}}` injects just " +
					"that value. Leave unset to use the whole secret.",
				Optional: true,
			},
			"engine": schema.StringAttribute{
				MarkdownDescription: "`builtin` when Watchpoint holds the encrypted value, otherwise the vendor " +
					"kind answering for this key.",
				Computed: true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Human-readable description of what this secret is for.",
				Optional:            true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: "Grouping category: `credential`, `token`, `certificate`, or `other`. Defaults to `credential`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("credential"),
			},
			"has_value": schema.BoolAttribute{
				MarkdownDescription: "Whether the secret currently has a stored value.",
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

// applyResponse copies API response fields into the model. The write-only
// `value` is intentionally left untouched.
func (m *vaultSecretModel) applyResponse(resp *vaultSecretResponse) {
	m.ID = types.StringValue(resp.ID)
	m.Key = types.StringValue(resp.Key)
	m.Description = strFromPtr(resp.Description)
	m.Category = strFromPtr(resp.Category)
	m.HasValue = types.BoolValue(resp.HasValue)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = strFromPtr(resp.UpdatedAt)

	if resp.Engine == "" {
		m.Engine = types.StringValue("builtin")
	} else {
		m.Engine = types.StringValue(resp.Engine)
	}

	// Unlike `value`, the reference IS refreshed: it holds nothing secret, so the API
	// returns it on every read and drift in it is worth surfacing. An empty optional
	// comes back as null rather than "" so a config that omits it does not diff forever.
	if resp.External == nil {
		m.ExternalVault = types.StringNull()
		m.ExternalName = types.StringNull()
		m.ExternalVersion = types.StringNull()
		m.ExternalField = types.StringNull()
		return
	}
	m.ExternalVault = strNullIfEmpty(resp.External.Connection)
	m.ExternalName = strNullIfEmpty(resp.External.Name)
	m.ExternalVersion = strNullIfEmpty(resp.External.Version)
	m.ExternalField = strNullIfEmpty(resp.External.Field)
}

// externalRef builds the reference to send, or nil when this secret stores a value
// here. Returning nil rather than an empty struct matters: the API treats an all-blank
// reference as "not supplied", and sending one on a metadata-only update would
// otherwise convert a perfectly good stored secret into a pointer at nothing.
func (m *vaultSecretModel) externalRef() *externalSecretRef {
	if m.ExternalVault.IsNull() || m.ExternalVault.IsUnknown() || m.ExternalVault.ValueString() == "" {
		return nil
	}
	return &externalSecretRef{
		Connection: m.ExternalVault.ValueString(),
		Name:       m.ExternalName.ValueString(),
		Version:    m.ExternalVersion.ValueString(),
		Field:      m.ExternalField.ValueString(),
	}
}

// ConfigValidators catches the two mistakes the API would otherwise reject at apply
// time, at plan time instead, where they are cheaper and the message points at the
// attribute rather than at an HTTP status.
func (r *vaultSecretResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		// A row with two sources of truth gives no way to say which one a check used.
		resourcevalidator.Conflicting(
			path.MatchRoot("value"),
			path.MatchRoot("external_vault"),
		),
		// The vault alone does not say WHICH secret in it.
		resourcevalidator.RequiredTogether(
			path.MatchRoot("external_vault"),
			path.MatchRoot("external_name"),
		),
	}
}

func (r *vaultSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vaultSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &vaultSecretCreateBody{
		Key:         plan.Key.ValueString(),
		Value:       strPtr(plan.Value),
		External:    plan.externalRef(),
		Description: strPtr(plan.Description),
		Category:    strPtr(plan.Category),
	}

	var created vaultSecretResponse
	path := r.client.Tenantf("/monitoring", "/vault")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating vault secret", err.Error())
		return
	}
	// The create response omits value (includeValue:false); leave plan.Value as
	// configured.
	plan.applyResponse(&created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vaultSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vaultSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got vaultSecretResponse
	getPath := r.client.Tenantf("/monitoring", "/vault/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading vault secret", err.Error())
		return
	}
	// Deliberately do not touch state.Value: it is write-only and refreshing it
	// (even though this GET returns the decrypted value) would churn state.
	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vaultSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan vaultSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &vaultSecretUpdateBody{
		// strPtr yields nil when value is null/unknown, so metadata-only updates
		// omit the value and leave the stored secret intact.
		Value:       strPtr(plan.Value),
		External:    plan.externalRef(),
		Description: strPtr(plan.Description),
		Category:    strPtr(plan.Category),
	}

	var got vaultSecretResponse
	path := r.client.Tenantf("/monitoring", "/vault/%s", plan.ID.ValueString())
	if err := r.client.Put(ctx, path, body, &got); err != nil {
		resp.Diagnostics.AddError("Error updating vault secret", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vaultSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vaultSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/monitoring", "/vault/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting vault secret", err.Error())
	}
}

func (r *vaultSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
