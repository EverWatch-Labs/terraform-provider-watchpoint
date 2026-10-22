package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// resource_acl is ONE subject's resource-scoped grant of ONE kind: "this role
// (or this member) may reach exactly these monitors". It maps onto Identity.Api
// AccessControlController, which stores one row per (kind, pattern, subject)
// and exposes only list, create and delete. There is no update endpoint, so
// every change here is a row delete plus a row create underneath.
//
// The declarative unit is the SET of patterns rather than a single row, because
// that is what makes the grant reconcilable. The contract in
// SharedPlatform.Common.Security.ResourceAcl is "no rows of a kind means that
// kind is not narrowed at all", so a grant Terraform could only ever add to
// would silently keep whatever a portal user pinned on the side, which for an
// access-control resource is the whole point of managing it here.
//
// patterns is a Set, never a List: the API returns rows ordered by kind then
// pattern, so a List would fail the apply with "Provider produced inconsistent
// result after apply" the first time someone wrote them in another order.
//
// Update adds the new rows BEFORE deleting the removed ones. Narrowing fails
// open by design, so deleting first would briefly WIDEN the subject's access
// instead of narrowing it.
//
// Exactly one of user_id or role_id names the subject. Both, and resource_kind,
// are part of the grant's identity and force replacement; so does label, which
// the API only accepts at row-create time. The id is
// "<resource_kind>/role/<role_id>" or "<resource_kind>/user/<user_id>", which is
// also the import id.
//
// An API key driving this needs the settings.manage scope as well as
// roles.manage: the access-control path matches no fragment in
// ApiKeyAuthMiddleware.PathScopeMap, so its writes fall through to the write
// default-deny, and the controller then enforces roles.manage of its own.

var (
	_ resource.Resource                = (*resourceAclResource)(nil)
	_ resource.ResourceWithConfigure   = (*resourceAclResource)(nil)
	_ resource.ResourceWithImportState = (*resourceAclResource)(nil)
)

// resourceAclKinds are the kinds ResourceAcl.GrantableKinds allows. Anything
// else is rejected by the API with a 400 naming this same list.
var resourceAclKinds = []string{"monitor", "dashboard", "status_page", "rum_site", "environment"}

// NewResourceAclResource is the resource constructor wired into the provider.
func NewResourceAclResource() resource.Resource { return &resourceAclResource{} }

type resourceAclResource struct {
	client *client.Client
}

// resourceAclModel maps schema attributes <-> Terraform state.
type resourceAclModel struct {
	ID           types.String `tfsdk:"id"`
	ResourceKind types.String `tfsdk:"resource_kind"`
	UserID       types.String `tfsdk:"user_id"`
	RoleID       types.String `tfsdk:"role_id"`
	Patterns     types.Set    `tfsdk:"patterns"`
	Label        types.String `tfsdk:"label"`
}

// --- Wire DTOs (camelCase JSON matching the identity API) ---

type resourceAclRow struct {
	ID           string  `json:"id"`
	ResourceKind string  `json:"resourceKind"`
	Pattern      string  `json:"pattern"`
	Label        *string `json:"label"`
	UserID       *string `json:"userId"`
	RoleID       *string `json:"roleId"`
}

type resourceAclCreateBody struct {
	ResourceKind string  `json:"resourceKind"`
	Pattern      string  `json:"pattern"`
	Label        *string `json:"label,omitempty"`
	UserID       *string `json:"userId,omitempty"`
	RoleID       *string `json:"roleId,omitempty"`
}

func (r *resourceAclResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource_acl"
}

func (r *resourceAclResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *resourceAclResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Pins one role or one member to specific monitors, dashboards, status pages or " +
			"RUM sites. A subject with no grants of a kind is not narrowed for that kind at all, so adding " +
			"the first grant is what switches narrowing on, and removing the last one switches it off again. " +
			"Tenant Owners and Admins are never narrowed. `patterns` is the complete set for this subject " +
			"and kind: a grant added outside Terraform is removed on the next apply. The id is " +
			"`<resource_kind>/role/<role_id>` or `<resource_kind>/user/<user_id>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Resource ACL grant ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"resource_kind": schema.StringAttribute{
				MarkdownDescription: "Kind of resource being narrowed: `" +
					strings.Join(resourceAclKinds, "`, `") + "`. Each kind is narrowed independently, " +
					"so granting monitors says nothing about dashboards. Changing it replaces the grant.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.OneOf(resourceAclKinds...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"role_id": schema.StringAttribute{
				MarkdownDescription: "ID of the tenant role the grant applies to. Exactly one of `role_id` " +
					"or `user_id` is required. The role must exist in this organization. Changing it " +
					"replaces the grant.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 128),
					stringvalidator.ExactlyOneOf(path.MatchRoot("role_id"), path.MatchRoot("user_id")),
				},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "ID of the member the grant applies to. Exactly one of `role_id` or " +
					"`user_id` is required. The member must be active in this organization. Changing it " +
					"replaces the grant.",
				Optional:      true,
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 128)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"patterns": schema.SetAttribute{
				MarkdownDescription: "The resources this subject may reach, as the complete set. Each entry " +
					"is a resource ID, a prefix glob such as `mon_payments-*`, or `*` for every resource of " +
					"the kind. Matching is case-insensitive. Removing an entry revokes it on the next apply.",
				Required:    true,
				ElementType: types.StringType,
				Validators:  []validator.Set{setvalidator.SizeAtLeast(1)},
			},
			"label": schema.StringAttribute{
				MarkdownDescription: "Optional display name shown beside the grant in the portal, which " +
					"otherwise falls back to the raw pattern. The API only accepts a label when the row is " +
					"created, so changing it replaces the grant. Never refreshed from the API.",
				Optional:      true,
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 200)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

// subjectKey returns the subject half of the synthetic id: "role"/"user" and
// the id. Exactly one of the two attributes is set, enforced by ExactlyOneOf.
func (m *resourceAclModel) subjectKey() (string, string) {
	if v := m.RoleID.ValueString(); v != "" {
		return "role", v
	}
	return "user", m.UserID.ValueString()
}

func (m *resourceAclModel) syntheticID() string {
	kind, id := m.subjectKey()
	return fmt.Sprintf("%s/%s/%s", m.ResourceKind.ValueString(), kind, id)
}

func (m *resourceAclModel) toCreateBody(pattern string) *resourceAclCreateBody {
	return &resourceAclCreateBody{
		ResourceKind: m.ResourceKind.ValueString(),
		Pattern:      pattern,
		Label:        strPtr(m.Label),
		UserID:       strPtr(m.UserID),
		RoleID:       strPtr(m.RoleID),
	}
}

// aclStr dereferences an optional wire string; the API sends null rather than
// "" for the subject it did not grant to.
func aclStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// isAclConflict reports a 409, which the API returns when the identical grant
// already exists. That is the state the configuration asks for, so it is not an
// error to create into.
func isAclConflict(err error) bool {
	var ae *client.APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusConflict
}

// matchResourceAclRows picks the rows for one subject and kind out of the
// tenant's whole ACL listing. Kind and subject id are compared
// case-insensitively because the API lower-cases the kind on write and resource
// ids vary in casing by producer, exactly as ResourceAcl.PatternMatches does.
func matchResourceAclRows(rows []resourceAclRow, kind, subjectKind, subjectID string) []resourceAclRow {
	out := make([]resourceAclRow, 0, len(rows))
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row.ResourceKind), strings.TrimSpace(kind)) {
			continue
		}
		got := aclStr(row.UserID)
		if subjectKind == "role" {
			got = aclStr(row.RoleID)
		}
		if got == "" || !strings.EqualFold(got, subjectID) {
			continue
		}
		out = append(out, row)
	}
	return out
}

// aclPatternsFromRows folds the stored rows into the set that goes to state. A
// stored pattern that only differs from the configured one by case or
// surrounding whitespace keeps the CONFIGURED spelling: the API trims on write,
// and writing a normalised value back into a Required attribute fails the apply
// with "was cty.StringVal(\"MON_1\"), but now cty.StringVal(\"mon_1\")". Anything
// else is real drift and is taken from the server so the next plan puts it back.
func aclPatternsFromRows(configured []string, rows []resourceAclRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		v := strings.TrimSpace(row.Pattern)
		for _, c := range configured {
			if strings.EqualFold(strings.TrimSpace(c), v) {
				v = c
				break
			}
		}
		out = append(out, v)
	}
	return out
}

// aclPatternDiff reports the patterns to create and the stored row ids to
// delete to reach the desired set. Case-insensitive, so a re-spelled pattern is
// not a needless revoke-and-regrant, and duplicate spellings of one pattern in
// the configuration collapse to a single create.
func aclPatternDiff(desired []string, rows []resourceAclRow) (add []string, removeIDs []string) {
	have := make(map[string]bool, len(rows))
	for _, row := range rows {
		have[strings.ToLower(strings.TrimSpace(row.Pattern))] = true
	}

	want := make(map[string]bool, len(desired))
	for _, p := range desired {
		t := strings.TrimSpace(p)
		k := strings.ToLower(t)
		if k == "" || want[k] {
			continue
		}
		want[k] = true
		if !have[k] {
			add = append(add, t)
		}
	}

	for _, row := range rows {
		if !want[strings.ToLower(strings.TrimSpace(row.Pattern))] {
			removeIDs = append(removeIDs, row.ID)
		}
	}
	return add, removeIDs
}

func (m *resourceAclModel) applyRows(ctx context.Context, rows []resourceAclRow, diags *diag.Diagnostics) {
	configured := stringSliceFromSet(ctx, m.Patterns, diags)
	m.ID = types.StringValue(m.syntheticID())
	m.Patterns = setFromStringSlice(ctx, aclPatternsFromRows(configured, rows), diags)
}

func (r *resourceAclResource) aclsPath() string {
	return r.client.Tenantf("/identity", "/access-control/acls")
}

// fetchRows lists the tenant's grants and keeps this subject's. The API returns
// the whole tenant listing with no filters, so the narrowing happens here.
func (r *resourceAclResource) fetchRows(ctx context.Context, m *resourceAclModel) ([]resourceAclRow, error) {
	var rows []resourceAclRow
	if err := r.client.Get(ctx, r.aclsPath(), nil, &rows); err != nil {
		return nil, err
	}
	subjectKind, subjectID := m.subjectKey()
	return matchResourceAclRows(rows, m.ResourceKind.ValueString(), subjectKind, subjectID), nil
}

func (r *resourceAclResource) createRow(ctx context.Context, m *resourceAclModel, pattern string) error {
	err := r.client.Post(ctx, r.aclsPath(), m.toCreateBody(pattern), nil)
	if isAclConflict(err) {
		return nil
	}
	return err
}

func (r *resourceAclResource) deleteRow(ctx context.Context, id string) error {
	err := r.client.Delete(ctx, r.client.Tenantf("/identity", "/access-control/acls/%s", id))
	if err != nil && !client.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *resourceAclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceAclModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	patterns := stringSliceFromSet(ctx, plan.Patterns, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, p := range patterns {
		if err := r.createRow(ctx, &plan, strings.TrimSpace(p)); err != nil {
			resp.Diagnostics.AddError("Error creating resource ACL", err.Error())
			return
		}
	}

	// Each create returns only its own row, so read the grant back whole.
	rows, err := r.fetchRows(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error reading back resource ACL", err.Error())
		return
	}
	if len(rows) == 0 {
		resp.Diagnostics.AddError("Error reading back resource ACL",
			"The grants were written but none are in the tenant's access-control listing.")
		return
	}

	plan.applyRows(ctx, rows, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resourceAclResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceAclModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rows, err := r.fetchRows(ctx, &state)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading resource ACL", err.Error())
		return
	}
	// Every row revoked outside Terraform means the grant no longer exists, and
	// the subject is no longer narrowed for this kind at all.
	if len(rows) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	state.applyRows(ctx, rows, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only ever runs for a patterns change; every other attribute forces
// replacement. Additions go in before revocations so the subject is never
// briefly un-narrowed, which is what a revoke-first order would do.
func (r *resourceAclResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceAclModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rows, err := r.fetchRows(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating resource ACL", err.Error())
		return
	}

	desired := stringSliceFromSet(ctx, plan.Patterns, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	add, removeIDs := aclPatternDiff(desired, rows)

	for _, p := range add {
		if err := r.createRow(ctx, &plan, p); err != nil {
			resp.Diagnostics.AddError("Error updating resource ACL", err.Error())
			return
		}
	}
	for _, id := range removeIDs {
		if err := r.deleteRow(ctx, id); err != nil {
			resp.Diagnostics.AddError("Error updating resource ACL", err.Error())
			return
		}
	}

	rows, err = r.fetchRows(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error reading back resource ACL", err.Error())
		return
	}

	plan.applyRows(ctx, rows, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resourceAclResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceAclModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rows, err := r.fetchRows(ctx, &state)
	if err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting resource ACL", err.Error())
		return
	}

	for _, row := range rows {
		if err := r.deleteRow(ctx, row.ID); err != nil {
			resp.Diagnostics.AddError("Error deleting resource ACL", err.Error())
			return
		}
	}
}

// ImportState takes "<resource_kind>/role/<role_id>" or
// "<resource_kind>/user/<user_id>" rather than a row id: one Terraform resource
// covers every row the subject holds of that kind, so no single row identifies
// it. Read fills in patterns and id.
func (r *resourceAclResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" || (parts[1] != "role" && parts[1] != "user") {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the form \"<resource_kind>/role/<role_id>\" or "+
				"\"<resource_kind>/user/<user_id>\", got: "+req.ID,
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource_kind"), parts[0])...)
	if parts[1] == "role" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), parts[2])...)
	} else {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[2])...)
	}
}
