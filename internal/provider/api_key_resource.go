package provider

import (
	"context"
	"fmt"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A tenant API key.
//
// ApiKeysController has no PATCH: name, scopes and expiry are fixed at creation,
// so each of those forces replacement. The two things that CAN change in place
// are the active flag (pause/resume) and the secret itself (roll), so Update
// drives those two sub-endpoints and nothing else.
//
// The plaintext key is returned exactly once, by Create and by Roll. It is
// therefore stored in state and marked Sensitive. There is no way to recover it
// afterwards, which is why `terraform import` can restore every attribute except
// `api_key` - see the note on that attribute and in docs/resources/api_key.md.

var (
	_ resource.Resource                = (*apiKeyResource)(nil)
	_ resource.ResourceWithConfigure   = (*apiKeyResource)(nil)
	_ resource.ResourceWithImportState = (*apiKeyResource)(nil)
)

// NewAPIKeyResource is the resource constructor wired into the provider.
func NewAPIKeyResource() resource.Resource { return &apiKeyResource{} }

type apiKeyResource struct {
	client *client.Client
}

// apiKeyModel maps schema attributes <-> Terraform state.
type apiKeyModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Scopes           types.Set    `tfsdk:"scopes"`
	ExpiresInDays    types.Int64  `tfsdk:"expires_in_days"`
	Enabled          types.Bool   `tfsdk:"enabled"`
	RotateTrigger    types.String `tfsdk:"rotate_trigger"`
	APIKey           types.String `tfsdk:"api_key"`
	KeyPrefix        types.String `tfsdk:"key_prefix"`
	CreatedAt        types.String `tfsdk:"created_at"`
	ExpiresAt        types.String `tfsdk:"expires_at"`
	LastUsedAt       types.String `tfsdk:"last_used_at"`
	AllowedIPRanges  types.Set    `tfsdk:"allowed_ip_ranges"`
	ServiceAccountID types.String `tfsdk:"service_account_id"`
}

// --- Wire DTOs (camelCase JSON matching the Identity API) ---

type apiKeyCreateBody struct {
	Name             string  `json:"name"`
	Scopes           *string `json:"scopes,omitempty"`
	ExpiresInDays    *int64  `json:"expiresInDays,omitempty"`
	AllowedIPRanges  *string `json:"allowedIpRanges,omitempty"`
	ServiceAccountID *string `json:"serviceAccountId,omitempty"`
}

// apiKeyCreatedResponse is returned by POST /api-keys and POST .../roll. It is
// the only place the plaintext key is ever exposed.
type apiKeyCreatedResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	KeyPrefix string  `json:"keyPrefix"`
	Scopes    string  `json:"scopes"`
	CreatedAt string  `json:"createdAt"`
	ExpiresAt *string `json:"expiresAt"`
	APIKey    string  `json:"apiKey"`
}

// apiKeyResponse is the list-shape. There is no GET by id on this controller,
// so Read lists the tenant's keys and selects by id.
type apiKeyResponse struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	KeyPrefix        string  `json:"keyPrefix"`
	Scopes           string  `json:"scopes"`
	IsActive         bool    `json:"isActive"`
	CreatedAt        string  `json:"createdAt"`
	ExpiresAt        *string `json:"expiresAt"`
	LastUsedAt       *string `json:"lastUsedAt"`
	RevokedAt        *string `json:"revokedAt"`
	CreatedByUserID  string  `json:"createdByUserId"`
	AllowedIPRanges  *string `json:"allowedIpRanges"`
	ServiceAccountID *string `json:"serviceAccountId"`
}

func (r *apiKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *apiKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A tenant API key. The plaintext secret is returned only at creation and on " +
			"rotation, so it is written to Terraform state and must be treated as a secret. `name`, `scopes` " +
			"and `expires_in_days` cannot be changed in place and replace the key; `enabled` and " +
			"`rotate_trigger` are applied without replacement. " +
			"The provider credential needs BOTH `apikeys.manage` (to create, rotate, pause and revoke) and " +
			"`settings.manage` (to list, which is how this resource reads state). A token holding only " +
			"`apikeys.manage` will create a key successfully and then fail on the next refresh.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "API key ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable name shown in the portal and on the audit trail.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"scopes": schema.SetAttribute{
				MarkdownDescription: "Scopes granted to the key, for example `monitors.read`. Required: the API " +
					"no longer defaults a blank list to full access. Use `[\"*\"]` only for a key that truly " +
					"needs everything, or `[\"scim\"]` for an IdP provisioning key. Unknown scopes are rejected " +
					"by the API against its scope catalog.",
				ElementType:   types.StringType,
				Required:      true,
				Validators:    []validator.Set{setvalidator.SizeAtLeast(1)},
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"allowed_ip_ranges": schema.SetAttribute{
				MarkdownDescription: "Per-key IPv4/IPv6 CIDR allowlist, applied on top of the tenant allowlist. " +
					"A request from any other address is refused with `403 ip_not_allowed`. Fixed at creation, " +
					"so changing it replaces the key.",
				ElementType:   types.StringType,
				Optional:      true,
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"service_account_id": schema.StringAttribute{
				MarkdownDescription: "Mint the key for a `watchpoint_service_account` instead of the calling " +
					"user. The key is then capped by the account's role whatever its scopes say, and stops " +
					"working when the account is deactivated or deleted.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"expires_in_days": schema.Int64Attribute{
				MarkdownDescription: "Lifetime in days from creation. Omit for a key that never expires. " +
					"This is a create-time input; the resulting timestamp is exported as `expires_at`.",
				Optional:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the key is currently usable. Setting this to `false` pauses the " +
					"key without revoking it, so it can be resumed later with the same secret. Defaults to `true`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"rotate_trigger": schema.StringAttribute{
				MarkdownDescription: "Arbitrary value that rotates the secret when it changes. Change it (for " +
					"example to a new date) to issue a fresh `api_key` in place, keeping the same `id`, name and " +
					"scopes. Rotation invalidates the previous secret immediately.",
				Optional: true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "The plaintext API key. Returned only at creation and on rotation, so it " +
					"is stored in state and anyone with state access can read it. It is **not** recoverable by " +
					"`terraform import`: an imported key has a null `api_key` until it is rotated.",
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key_prefix": schema.StringAttribute{
				MarkdownDescription: "Truncated, non-secret prefix of the key, safe to log or display.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"expires_at": schema.StringAttribute{
				MarkdownDescription: "Expiry timestamp (RFC 3339), or null when the key does not expire.",
				Computed:            true,
			},
			"last_used_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the last authenticated request made with this key, or null " +
					"when it has never been used.",
				Computed: true,
			},
		},
	}
}

// applyCreated maps the create/roll response, which is the only shape that
// carries the plaintext secret.
func (m *apiKeyModel) applyCreated(ctx context.Context, in *apiKeyCreatedResponse, ds *diagSink) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.Scopes = setFromCSVPtr(ctx, &in.Scopes, ds.d)
	m.APIKey = types.StringValue(in.APIKey)
	m.KeyPrefix = types.StringValue(in.KeyPrefix)
	m.CreatedAt = types.StringValue(in.CreatedAt)
	m.ExpiresAt = strFromPtr(in.ExpiresAt)
}

// applyRead maps the list shape. It deliberately leaves api_key untouched: the
// API cannot return it again, so whatever is in state (the secret, or null for
// an imported key) is the best available value.
func (m *apiKeyModel) applyRead(ctx context.Context, in *apiKeyResponse, ds *diagSink) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.Scopes = setFromCSVPtr(ctx, &in.Scopes, ds.d)
	m.Enabled = types.BoolValue(in.IsActive)
	m.KeyPrefix = types.StringValue(in.KeyPrefix)
	m.CreatedAt = types.StringValue(in.CreatedAt)
	m.ExpiresAt = strFromPtr(in.ExpiresAt)
	m.LastUsedAt = strFromPtr(in.LastUsedAt)
	m.AllowedIPRanges = setFromCSVPtr(ctx, in.AllowedIPRanges, ds.d)
	m.ServiceAccountID = strFromPtr(in.ServiceAccountID)
}

// fetch lists the tenant's keys and returns the one matching id. The controller
// exposes no GET by id, so this is the only read path available.
func (r *apiKeyResource) fetch(ctx context.Context, id string) (*apiKeyResponse, error) {
	var keys []apiKeyResponse
	path := r.client.Tenantf("/identity", "/api-keys")
	if err := r.client.Get(ctx, path, nil, &keys); err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].ID == id {
			// A revoked key still appears in the list. Treat it as gone so
			// Terraform recreates it rather than reporting healthy drift.
			if keys[i].RevokedAt != nil {
				return nil, nil
			}
			return &keys[i], nil
		}
	}
	return nil, nil
}

func (r *apiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	body := &apiKeyCreateBody{
		Name:             plan.Name.ValueString(),
		Scopes:           csvPtrFromSet(ctx, plan.Scopes, &resp.Diagnostics),
		ExpiresInDays:    int64Ptr(plan.ExpiresInDays),
		AllowedIPRanges:  csvPtrFromSet(ctx, plan.AllowedIPRanges, &resp.Diagnostics),
		ServiceAccountID: strPtr(plan.ServiceAccountID),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var created apiKeyCreatedResponse
	path := r.client.Tenantf("/identity", "/api-keys")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating API key", err.Error())
		return
	}

	plan.applyCreated(ctx, &created, ds)
	// A key is created active. Honour enabled=false by pausing straight away, so
	// the resource lands in the state the configuration asked for.
	if !plan.Enabled.IsNull() && !plan.Enabled.ValueBool() {
		if err := r.setEnabled(ctx, created.ID, false); err != nil {
			resp.Diagnostics.AddError("Error pausing new API key", err.Error())
			return
		}
	}
	plan.LastUsedAt = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.fetch(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading API key", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	ds := &diagSink{d: &resp.Diagnostics}
	state.applyRead(ctx, got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// setEnabled drives the pause/resume sub-endpoints.
func (r *apiKeyResource) setEnabled(ctx context.Context, id string, enabled bool) error {
	action := "pause"
	if enabled {
		action = "resume"
	}
	path := r.client.Tenantf("/identity", "/api-keys/%s/%s", id, action)
	return r.client.Post(ctx, path, nil, nil)
}

// Update handles the only two in-place changes the API supports: pause/resume,
// and rotation. Every other attribute forces replacement, so the framework never
// reaches this function for them.
func (r *apiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state apiKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	plan.ID = state.ID
	plan.APIKey = state.APIKey
	plan.KeyPrefix = state.KeyPrefix
	plan.CreatedAt = state.CreatedAt
	plan.ExpiresAt = state.ExpiresAt
	plan.LastUsedAt = state.LastUsedAt

	// Rotate first: a paused key cannot be rolled, so if this apply both resumes
	// and rotates, the resume has to happen before the roll. Doing the rotation
	// first would fail on a key that is currently paused.
	rotating := !plan.RotateTrigger.Equal(state.RotateTrigger)
	resuming := plan.Enabled.ValueBool() && !state.Enabled.ValueBool()

	if resuming {
		if err := r.setEnabled(ctx, id, true); err != nil {
			resp.Diagnostics.AddError("Error resuming API key", err.Error())
			return
		}
		plan.Enabled = types.BoolValue(true)
	}

	if rotating {
		ds := &diagSink{d: &resp.Diagnostics}
		var rolled apiKeyCreatedResponse
		path := r.client.Tenantf("/identity", "/api-keys/%s/roll", id)
		if err := r.client.Post(ctx, path, nil, &rolled); err != nil {
			resp.Diagnostics.AddError("Error rotating API key",
				fmt.Sprintf("%s. A revoked key cannot be rotated; recreate it instead.", err.Error()))
			return
		}
		plan.applyCreated(ctx, &rolled, ds)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Pause last, so a single apply that both rotates and pauses ends paused.
	if !plan.Enabled.ValueBool() && state.Enabled.ValueBool() {
		if err := r.setEnabled(ctx, id, false); err != nil {
			resp.Diagnostics.AddError("Error pausing API key", err.Error())
			return
		}
		plan.Enabled = types.BoolValue(false)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/identity", "/api-keys/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error revoking API key", err.Error())
	}
}

func (r *apiKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
