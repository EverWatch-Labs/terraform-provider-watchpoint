package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An audit-log webhook destination (AuditWebhooksController). The HMAC signing
// secret is generated server-side and returned ONLY in the create response; the
// list endpoint never includes it. It is therefore Computed+Sensitive, captured
// at create, and preserved across reads. There is no GET-by-id; Read lists the
// tenant's destinations (bare array) and matches on id. Update is a PATCH.

var (
	_ resource.Resource                = (*auditWebhookResource)(nil)
	_ resource.ResourceWithConfigure   = (*auditWebhookResource)(nil)
	_ resource.ResourceWithImportState = (*auditWebhookResource)(nil)
)

// NewAuditWebhookResource is the resource constructor wired into the provider.
func NewAuditWebhookResource() resource.Resource { return &auditWebhookResource{} }

type auditWebhookResource struct {
	client *client.Client
}

// auditWebhookModel maps schema attributes <-> Terraform state.
type auditWebhookModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	URL           types.String `tfsdk:"url"`
	IsActive      types.Bool   `tfsdk:"is_active"`
	SigningSecret types.String `tfsdk:"signing_secret"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

// --- Wire DTOs (camelCase JSON matching the Identity API) ---

type auditWebhookCreateBody struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type auditWebhookUpdateBody struct {
	Name     *string `json:"name,omitempty"`
	URL      *string `json:"url,omitempty"`
	IsActive *bool   `json:"isActive,omitempty"`
}

// auditWebhookCreateResponse is the one-time create response carrying the secret.
type auditWebhookCreateResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	IsActive      bool   `json:"isActive"`
	SigningSecret string `json:"signingSecret"`
}

// auditWebhookListItem is one element of the list response (no secret).
type auditWebhookListItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	IsActive  bool   `json:"isActive"`
	CreatedAt string `json:"createdAt"`
}

func (r *auditWebhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_audit_webhook"
}

func (r *auditWebhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *auditWebhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An audit-log webhook destination. The signing secret is returned only at " +
			"creation; it is stored in state and never re-read from the API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Webhook destination ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name for the destination.",
				Required:            true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "HTTPS URL audit-log events are POSTed to.",
				Required:            true,
			},
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Whether deliveries are active. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"signing_secret": schema.StringAttribute{
				MarkdownDescription: "HMAC signing secret used to verify delivered payloads. Returned only at " +
					"creation and kept in state thereafter.",
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

// applyListItem copies list-response fields into the model. The signing secret
// is never present here, so it is left untouched.
func (m *auditWebhookModel) applyListItem(item *auditWebhookListItem) {
	m.ID = types.StringValue(item.ID)
	m.Name = types.StringValue(item.Name)
	m.URL = types.StringValue(item.URL)
	m.IsActive = types.BoolValue(item.IsActive)
	m.CreatedAt = types.StringValue(item.CreatedAt)
}

func (r *auditWebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan auditWebhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &auditWebhookCreateBody{
		Name: plan.Name.ValueString(),
		URL:  plan.URL.ValueString(),
	}
	var created auditWebhookCreateResponse
	path := r.client.Tenantf("/identity", "/audit-webhooks")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating audit webhook", err.Error())
		return
	}

	// Capture the one-time secret now; it is never returned again.
	plan.ID = types.StringValue(created.ID)
	plan.SigningSecret = types.StringValue(created.SigningSecret)

	// Apply requested is_active (create response reflects the server default).
	if err := r.applyDesiredActive(ctx, &plan, created.IsActive); err != nil {
		resp.Diagnostics.AddError("Error setting audit webhook active state", err.Error())
		return
	}

	found, err := r.findWebhookByID(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading audit webhook after create", err.Error())
		return
	}
	if found != nil {
		secret := plan.SigningSecret
		plan.applyListItem(found)
		plan.SigningSecret = secret
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *auditWebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state auditWebhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := r.findWebhookByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading audit webhook", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Preserve the signing secret: the list endpoint never returns it.
	secret := state.SigningSecret
	state.applyListItem(found)
	state.SigningSecret = secret
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *auditWebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan auditWebhookModel
	var state auditWebhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &auditWebhookUpdateBody{
		Name:     strPtr(plan.Name),
		URL:      strPtr(plan.URL),
		IsActive: boolPtr(plan.IsActive),
	}
	path := r.client.Tenantf("/identity", "/audit-webhooks/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating audit webhook", err.Error())
		return
	}

	found, err := r.findWebhookByID(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading audit webhook after update", err.Error())
		return
	}
	// The secret is not user-settable and survives across updates.
	plan.SigningSecret = state.SigningSecret
	if found != nil {
		secret := plan.SigningSecret
		plan.applyListItem(found)
		plan.SigningSecret = secret
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *auditWebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state auditWebhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/identity", "/audit-webhooks/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting audit webhook", err.Error())
	}
}

func (r *auditWebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}

// applyDesiredActive PATCHes is_active when the plan differs from the value the
// create endpoint applied (create only accepts name+url, defaulting active=true).
func (r *auditWebhookResource) applyDesiredActive(ctx context.Context, plan *auditWebhookModel, createdActive bool) error {
	if plan.IsActive.IsNull() || plan.IsActive.IsUnknown() {
		return nil
	}
	if plan.IsActive.ValueBool() == createdActive {
		return nil
	}
	body := &auditWebhookUpdateBody{IsActive: boolPtr(plan.IsActive)}
	path := r.client.Tenantf("/identity", "/audit-webhooks/%s", plan.ID.ValueString())
	return r.client.Patch(ctx, path, body, nil)
}

// findWebhookByID lists the tenant's audit webhooks (bare array) and returns the
// one matching id, or nil when not present.
func (r *auditWebhookResource) findWebhookByID(ctx context.Context, id string) (*auditWebhookListItem, error) {
	var list []auditWebhookListItem
	path := r.client.Tenantf("/identity", "/audit-webhooks")
	if err := r.client.Get(ctx, path, nil, &list); err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, nil
}
