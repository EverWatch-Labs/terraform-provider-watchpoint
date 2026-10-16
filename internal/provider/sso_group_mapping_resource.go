package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An SSO group mapping (SsoController group-mappings sub-endpoints). A mapping
// ties an SSO group claim to a local group within one SSO configuration.
// config_id and sso_group_claim are immutable (RequiresReplace); group_id is
// updatable via PUT. There is no GET-by-id; Read lists the config's mappings and
// matches on id. The list response is shaped {mappings:[...], unmapped:[...]}.

var (
	_ resource.Resource                = (*ssoGroupMappingResource)(nil)
	_ resource.ResourceWithConfigure   = (*ssoGroupMappingResource)(nil)
	_ resource.ResourceWithImportState = (*ssoGroupMappingResource)(nil)
)

// NewSSOGroupMappingResource is the resource constructor wired into the provider.
func NewSSOGroupMappingResource() resource.Resource { return &ssoGroupMappingResource{} }

type ssoGroupMappingResource struct {
	client *client.Client
}

// ssoGroupMappingModel maps schema attributes <-> Terraform state.
type ssoGroupMappingModel struct {
	ID            types.String `tfsdk:"id"`
	ConfigID      types.String `tfsdk:"config_id"`
	SSOGroupClaim types.String `tfsdk:"sso_group_claim"`
	GroupID       types.String `tfsdk:"group_id"`
}

// --- Wire DTOs (camelCase JSON matching the Identity API) ---

type ssoGroupMappingCreateBody struct {
	SSOGroupClaim string  `json:"ssoGroupClaim"`
	GroupID       *string `json:"groupId,omitempty"`
}

type ssoGroupMappingUpdateBody struct {
	GroupID *string `json:"groupId,omitempty"`
}

type ssoGroupMappingResponse struct {
	ID            string  `json:"id"`
	SSOGroupClaim string  `json:"ssoGroupClaim"`
	GroupID       *string `json:"groupId"`
}

type ssoGroupMappingListResponse struct {
	Mappings []ssoGroupMappingResponse `json:"mappings"`
}

func (r *ssoGroupMappingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sso_group_mapping"
}

func (r *ssoGroupMappingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *ssoGroupMappingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Maps an SSO group claim to a local user group within an SSO configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Group mapping ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"config_id": schema.StringAttribute{
				MarkdownDescription: "ID of the SSO configuration this mapping belongs to.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"sso_group_claim": schema.StringAttribute{
				MarkdownDescription: "The SSO group claim value to match (e.g. an IdP group name or ID).",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "ID of the local user group to assign. May be omitted to leave it unmapped.",
				Optional:            true,
			},
		},
	}
}

// applyResponse copies API response fields into the model. config_id is not
// returned by the API and is preserved from plan/state by the caller.
func (m *ssoGroupMappingModel) applyResponse(resp *ssoGroupMappingResponse) {
	m.ID = types.StringValue(resp.ID)
	m.SSOGroupClaim = types.StringValue(resp.SSOGroupClaim)
	m.GroupID = strFromPtr(resp.GroupID)
}

func (r *ssoGroupMappingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ssoGroupMappingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &ssoGroupMappingCreateBody{
		SSOGroupClaim: plan.SSOGroupClaim.ValueString(),
		GroupID:       strPtr(plan.GroupID),
	}
	var created idNameResponse
	path := r.client.Tenantf("/identity", "/sso/%s/group-mappings", plan.ConfigID.ValueString())
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating SSO group mapping", err.Error())
		return
	}

	found, err := r.findMappingByID(ctx, plan.ConfigID.ValueString(), created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading SSO group mapping after create", err.Error())
		return
	}
	if found == nil {
		resp.Diagnostics.AddError("Error reading SSO group mapping after create",
			fmt.Sprintf("mapping %s not found after create", created.ID))
		return
	}
	plan.applyResponse(found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ssoGroupMappingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ssoGroupMappingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := r.findMappingByID(ctx, state.ConfigID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading SSO group mapping", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.applyResponse(found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ssoGroupMappingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ssoGroupMappingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := &ssoGroupMappingUpdateBody{GroupID: strPtr(plan.GroupID)}
	path := r.client.Tenantf("/identity", "/sso/%s/group-mappings/%s",
		plan.ConfigID.ValueString(), plan.ID.ValueString())
	if err := r.client.Put(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating SSO group mapping", err.Error())
		return
	}

	found, err := r.findMappingByID(ctx, plan.ConfigID.ValueString(), plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading SSO group mapping after update", err.Error())
		return
	}
	if found != nil {
		plan.applyResponse(found)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ssoGroupMappingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ssoGroupMappingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/identity", "/sso/%s/group-mappings/%s",
		state.ConfigID.ValueString(), state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting SSO group mapping", err.Error())
	}
}

// ImportState accepts a composite "configId/mappingId" identifier because the
// mapping cannot be located without knowing its parent SSO configuration.
func (r *ssoGroupMappingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected import ID in the form \"configId/mappingId\", got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("config_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// findMappingByID lists the config's group mappings and returns the one matching
// id, or nil when not present.
func (r *ssoGroupMappingResource) findMappingByID(ctx context.Context, configID, id string) (*ssoGroupMappingResponse, error) {
	var list ssoGroupMappingListResponse
	path := r.client.Tenantf("/identity", "/sso/%s/group-mappings", configID)
	if err := r.client.Get(ctx, path, nil, &list); err != nil {
		return nil, err
	}
	for i := range list.Mappings {
		if list.Mappings[i].ID == id {
			return &list.Mappings[i], nil
		}
	}
	return nil, nil
}
