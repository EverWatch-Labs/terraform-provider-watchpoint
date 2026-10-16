package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A bounded-use agent enrollment token (TenantAgentsController, Region API).
//
// The API mints (POST), lists live tokens (GET) and revokes (DELETE); there is
// no update and no GET by id, so every input forces replacement.
//
// Read deliberately does NOT call the list endpoint. That list drops a token as
// soon as it is spent, expired or revoked, and those are indistinguishable from
// each other. Removing the resource on a miss would mint a fresh token on the
// next apply after every successful install (or after the default 7 days), and
// replace anything that consumes `token`, such as a VM's user_data. A spent
// token is the normal end of its lifecycle, so state is kept as created. Use
// `terraform apply -replace` or `replace_triggered_by` to mint a new one.

var (
	_ resource.Resource              = (*agentEnrollmentTokenResource)(nil)
	_ resource.ResourceWithConfigure = (*agentEnrollmentTokenResource)(nil)
)

// agentRoles mirrors AgentCredential.KnownRoles on the server.
var agentRoles = []string{"private-location", "infrastructure-agent", "kubernetes"}

// NewAgentEnrollmentTokenResource is the resource constructor wired into the provider.
func NewAgentEnrollmentTokenResource() resource.Resource { return &agentEnrollmentTokenResource{} }

type agentEnrollmentTokenResource struct {
	client *client.Client
}

type agentEnrollmentTokenModel struct {
	ID              types.String `tfsdk:"id"`
	Roles           types.Set    `tfsdk:"roles"`
	LocationID      types.String `tfsdk:"location_id"`
	LocationName    types.String `tfsdk:"location_name"`
	MaxUses         types.Int64  `tfsdk:"max_uses"`
	ExpiresInHours  types.Int64  `tfsdk:"expires_in_hours"`
	AutoUpdate      types.Bool   `tfsdk:"auto_update"`
	Token           types.String `tfsdk:"token"`
	ExpiresAt       types.String `tfsdk:"expires_at"`
	InstallCommands types.Map    `tfsdk:"install_commands"`
}

// --- Wire DTOs (camelCase JSON matching TenantAgentsController) ---

type agentEnrollmentTokenCreateBody struct {
	Roles          []string `json:"roles"`
	LocationID     *string  `json:"locationId,omitempty"`
	LocationName   *string  `json:"locationName,omitempty"`
	MaxUses        int64    `json:"maxUses"`
	ExpiresInHours *int64   `json:"expiresInHours,omitempty"`
	AutoUpdate     bool     `json:"autoUpdate"`
}

// agentEnrollmentTokenCreatedResponse is the only shape that carries the
// plaintext token; the platform stores its SHA-256 and cannot show it again.
type agentEnrollmentTokenCreatedResponse struct {
	TokenID         string             `json:"tokenId"`
	Token           string             `json:"token"`
	ExpiresAt       string             `json:"expiresAt"`
	MaxUses         int64              `json:"maxUses"`
	Roles           []string           `json:"roles"`
	AutoUpdate      bool               `json:"autoUpdate"`
	LocationID      *string            `json:"locationId"`
	InstallCommands map[string]*string `json:"installCommands"`
}

func (r *agentEnrollmentTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent_enrollment_token"
}

func (r *agentEnrollmentTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *agentEnrollmentTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A bounded-use token that enrolls a Watchpoint agent (private location, " +
			"infrastructure agent or Kubernetes operator). The plaintext token is returned only at creation, " +
			"so it is written to Terraform state and must be treated as a secret. Every argument is fixed at " +
			"creation and replaces the token when changed. Destroying the resource revokes the token. " +
			"A token that has been used up or has expired is kept in state as created rather than recreated, " +
			"so dependent resources are not replaced after a successful install; use `terraform apply -replace` " +
			"to mint a new one. The provider credential needs the `profiles.manage` permission.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Enrollment token ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"roles": schema.SetAttribute{
				MarkdownDescription: "Roles the enrolling agent may claim: `private-location`, " +
					"`infrastructure-agent` and/or `kubernetes`.",
				ElementType: types.StringType,
				Required:    true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.OneOf(agentRoles...)),
				},
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"location_id": schema.StringAttribute{
				MarkdownDescription: "Existing private location to join. A `private-location` role needs " +
					"either this or `location_name`.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"location_name": schema.StringAttribute{
				MarkdownDescription: "Name of the private location to create (or join, if one with this name " +
					"already exists) when the agent enrolls.",
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"max_uses": schema.Int64Attribute{
				MarkdownDescription: "How many agents can enroll with this token, from 1 to 50. Defaults to `1`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
				Validators:          []validator.Int64{int64validator.Between(1, 50)},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"expires_in_hours": schema.Int64Attribute{
				MarkdownDescription: "Lifetime in hours from creation, from 1 to 720. Omit for the platform " +
					"default of 7 days. The resulting timestamp is exported as `expires_at`.",
				Optional:      true,
				Validators:    []validator.Int64{int64validator.Between(1, 720)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"auto_update": schema.BoolAttribute{
				MarkdownDescription: "Whether agents installed with the returned `install_commands` update " +
					"themselves. Only affects the generated commands, not the token. Defaults to `true`.",
				Optional:      true,
				Computed:      true,
				Default:       booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "The plaintext enrollment token. Returned only at creation, so it is " +
					"stored in state and anyone with state access can read it.",
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"expires_at": schema.StringAttribute{
				MarkdownDescription: "Expiry timestamp (RFC 3339).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"install_commands": schema.MapAttribute{
				MarkdownDescription: "Ready-to-run install commands with the token and control-plane host " +
					"already substituted, keyed by `linux`, `windows`, `macos` and, for a `kubernetes` token, " +
					"`helm`. Sensitive because each one embeds the token.",
				ElementType:   types.StringType,
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

// applyCreated maps the mint response onto state. Inputs the response echoes
// back stay as planned, so the plan and state cannot disagree.
func (m *agentEnrollmentTokenModel) applyCreated(ctx context.Context, in *agentEnrollmentTokenCreatedResponse, ds *diagSink) {
	m.ID = types.StringValue(in.TokenID)
	m.Token = types.StringValue(in.Token)
	m.ExpiresAt = types.StringValue(in.ExpiresAt)
	cmds := map[string]string{}
	for k, v := range in.InstallCommands {
		// helm is null for a token without the kubernetes role.
		if v != nil && *v != "" {
			cmds[k] = *v
		}
	}
	mv, d := types.MapValueFrom(ctx, types.StringType, cmds)
	ds.d.Append(d...)
	m.InstallCommands = mv
}

func (r *agentEnrollmentTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan agentEnrollmentTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &agentEnrollmentTokenCreateBody{
		Roles:          stringSliceFromSet(ctx, plan.Roles, &resp.Diagnostics),
		LocationID:     strPtr(plan.LocationID),
		LocationName:   strPtr(plan.LocationName),
		MaxUses:        plan.MaxUses.ValueInt64(),
		ExpiresInHours: int64Ptr(plan.ExpiresInHours),
		AutoUpdate:     plan.AutoUpdate.ValueBool(),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var created agentEnrollmentTokenCreatedResponse
	path := r.client.Tenantf("/regions", "/agents/enrollment-tokens")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating agent enrollment token", err.Error())
		return
	}

	plan.applyCreated(ctx, &created, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read keeps state as created. See the file comment for why the list endpoint
// is not consulted.
func (r *agentEnrollmentTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state agentEnrollmentTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never reached for a real change: every argument forces
// replacement. It carries the plan through so the framework contract holds.
func (r *agentEnrollmentTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state agentEnrollmentTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID, plan.Token, plan.ExpiresAt, plan.InstallCommands = state.ID, state.Token, state.ExpiresAt, state.InstallCommands
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete revokes the token. The endpoint is idempotent and also accepts a spent
// or expired token, so only a 404 (already gone) is ignored.
func (r *agentEnrollmentTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state agentEnrollmentTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/regions", "/agents/enrollment-tokens/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error revoking agent enrollment token", err.Error())
	}
}
