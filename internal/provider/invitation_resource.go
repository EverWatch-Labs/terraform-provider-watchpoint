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

// A tenant invitation (TenantsController invite/invitations endpoints).
// Create-and-delete only: there is no update endpoint, so the single
// user-settable attribute (email) forces replacement and Update is a
// passthrough. The invite POST only accepts an email - the role, expiry and
// other knobs the UI exposes are server-defaulted (default Member role, 7-day
// expiry) and surfaced here as computed values. There is no GET-by-id; Read
// lists the tenant's invitations (bare array) and matches on id.

var (
	_ resource.Resource                = (*invitationResource)(nil)
	_ resource.ResourceWithConfigure   = (*invitationResource)(nil)
	_ resource.ResourceWithImportState = (*invitationResource)(nil)
)

// NewInvitationResource is the resource constructor wired into the provider.
func NewInvitationResource() resource.Resource { return &invitationResource{} }

type invitationResource struct {
	client *client.Client
}

// invitationModel maps schema attributes <-> Terraform state.
type invitationModel struct {
	ID              types.String `tfsdk:"id"`
	Email           types.String `tfsdk:"email"`
	Status          types.String `tfsdk:"status"`
	CreatedAt       types.String `tfsdk:"created_at"`
	ExpiresAt       types.String `tfsdk:"expires_at"`
	AcceptedAt      types.String `tfsdk:"accepted_at"`
	InvitedByUserID types.String `tfsdk:"invited_by_user_id"`
}

// --- Wire DTOs (camelCase JSON matching the Identity API) ---

type invitationCreateBody struct {
	Email string `json:"email"`
}

// invitationCreateResponse is the invite POST response shape.
type invitationCreateResponse struct {
	Message      string `json:"message"`
	InvitationID string `json:"invitationId"`
}

// invitationListItem is one element of the invitations list response. status is
// returned as an int enum, so it is decoded into a flexible field.
type invitationListItem struct {
	ID              string  `json:"id"`
	Email           string  `json:"email"`
	Status          int     `json:"status"`
	CreatedAt       string  `json:"createdAt"`
	ExpiresAt       *string `json:"expiresAt"`
	AcceptedAt      *string `json:"acceptedAt"`
	InvitedByUserID *string `json:"invitedByUserId"`
}

// invitationStatusName maps the InvitationStatus enum (Pending, Accepted,
// Expired, Revoked) onto its string form for stable, human-readable state.
func invitationStatusName(v int) string {
	switch v {
	case 0:
		return "Pending"
	case 1:
		return "Accepted"
	case 2:
		return "Expired"
	case 3:
		return "Revoked"
	default:
		return "Unknown"
	}
}

func (r *invitationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_invitation"
}

func (r *invitationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *invitationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Invites a user to the tenant by email. The invitation cannot be edited in place; " +
			"changing the email replaces it. Deleting the resource revokes the invitation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Invitation ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "Email address to invite.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Invitation status (`Pending`, `Accepted`, `Expired`, `Revoked`).",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
			"expires_at": schema.StringAttribute{
				MarkdownDescription: "Expiry timestamp (RFC 3339).",
				Computed:            true,
			},
			"accepted_at": schema.StringAttribute{
				MarkdownDescription: "Acceptance timestamp (RFC 3339), if accepted.",
				Computed:            true,
			},
			"invited_by_user_id": schema.StringAttribute{
				MarkdownDescription: "ID of the user who sent the invitation.",
				Computed:            true,
			},
		},
	}
}

// applyListItem copies list-response fields into the model.
func (m *invitationModel) applyListItem(item *invitationListItem) {
	m.ID = types.StringValue(item.ID)
	m.Email = types.StringValue(item.Email)
	m.Status = types.StringValue(invitationStatusName(item.Status))
	m.CreatedAt = types.StringValue(item.CreatedAt)
	m.ExpiresAt = strFromPtr(item.ExpiresAt)
	m.AcceptedAt = strFromPtr(item.AcceptedAt)
	m.InvitedByUserID = strFromPtr(item.InvitedByUserID)
}

func (r *invitationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan invitationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &invitationCreateBody{Email: plan.Email.ValueString()}
	var created invitationCreateResponse
	path := r.client.Tenantf("/identity", "/invite")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating invitation", err.Error())
		return
	}

	found, err := r.findInvitationByID(ctx, created.InvitationID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading invitation after create", err.Error())
		return
	}
	if found == nil {
		// Fall back to the create response if the list hasn't caught up.
		plan.ID = types.StringValue(created.InvitationID)
	} else {
		plan.applyListItem(found)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *invitationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state invitationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := r.findInvitationByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading invitation", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.applyListItem(found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a passthrough: the only user-settable attribute forces replacement.
func (r *invitationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan invitationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *invitationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state invitationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/identity", "/invitations/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting invitation", err.Error())
	}
}

func (r *invitationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}

// findInvitationByID lists the tenant's invitations (bare array) and returns the
// one matching id, or nil when not present.
func (r *invitationResource) findInvitationByID(ctx context.Context, id string) (*invitationListItem, error) {
	var list []invitationListItem
	path := r.client.Tenantf("/identity", "/invitations")
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
