package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// cost_allocation_tag maps ONE tenant resource to one cost-allocation tag
// key/value, which is the unit the Billing API stores and the showback report
// apportions spend across.
//
// There is deliberately no set-of-resource-ids attribute here: a mapping is
// unique on (tag_key, resource_type, resource_id) server-side, so one resource
// per mapping is what the API models, and fanning a tag across a fleet is
// Terraform's own `for_each` rather than a collection the provider has to diff.
// (If a set is ever added here it must be a Set, never a List: the API returns
// mappings ordered by key/value/resource id, which fails a List round-trip with
// "Provider produced inconsistent result after apply".)
//
// Only tag_value is mutable; the other three identify the row, so they force
// replacement.

var (
	_ resource.Resource                = (*costAllocationTagResource)(nil)
	_ resource.ResourceWithConfigure   = (*costAllocationTagResource)(nil)
	_ resource.ResourceWithImportState = (*costAllocationTagResource)(nil)
)

// costAllocationResourceTypes are the billing ProductCategory names the API
// accepts and echoes back. Sent and compared verbatim.
var costAllocationResourceTypes = []string{
	"Monitor", "Certificate", "Synthetic", "Storage", "Network",
	"ApiCalls", "StatusPage", "Integration", "Team", "Agent",
}

// NewCostAllocationTagResource is the resource constructor wired into the provider.
func NewCostAllocationTagResource() resource.Resource { return &costAllocationTagResource{} }

type costAllocationTagResource struct {
	client *client.Client
}

// costAllocationTagModel maps schema attributes <-> Terraform state.
type costAllocationTagModel struct {
	ID           types.String `tfsdk:"id"`
	TagKey       types.String `tfsdk:"tag_key"`
	TagValue     types.String `tfsdk:"tag_value"`
	ResourceType types.String `tfsdk:"resource_type"`
	ResourceID   types.String `tfsdk:"resource_id"`
}

// --- Wire DTOs (camelCase JSON matching the billing API) ---

type costTagMapping struct {
	TagKey       string `json:"tagKey"`
	TagValue     string `json:"tagValue"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
}

type costTagUpsertBody struct {
	Mappings []costTagMapping `json:"mappings"`
}

type costTagUpsertResponse struct {
	Written int `json:"written"`
}

type costTagRow struct {
	ID           string `json:"id"`
	TagKey       string `json:"tagKey"`
	TagValue     string `json:"tagValue"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
}

type costTagListResponse struct {
	Items []costTagRow `json:"items"`
}

func (r *costAllocationTagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cost_allocation_tag"
}

func (r *costAllocationTagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *costAllocationTagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A cost-allocation tag on one billable resource. The showback report divides each " +
			"product's spend across the values of a tag key in proportion to how many resources in that " +
			"product's category carry each value, so tagging the fleet under a key like `team` is what makes " +
			"a chargeback report add up. A resource carries at most one value per key. The id is " +
			"`<tag_key>/<resource_type>/<resource_id>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Cost-allocation mapping ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"tag_key": schema.StringAttribute{
				MarkdownDescription: "Tag key, for example `team`, `env` or `cost-center`. Stored lower-cased, " +
					"so `Team` and `team` are the same key. Changing it replaces the mapping.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 64)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"tag_value": schema.StringAttribute{
				MarkdownDescription: "Tag value the resource is allocated to, for example `payments`. " +
					"Leading and trailing whitespace is trimmed.",
				Required:   true,
				Validators: []validator.String{stringvalidator.LengthBetween(1, 128)},
			},
			"resource_type": schema.StringAttribute{
				MarkdownDescription: "Billing product category the resource belongs to: `" +
					strings.Join(costAllocationResourceTypes, "`, `") + "`. Changing it replaces the mapping.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.OneOf(costAllocationResourceTypes...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"resource_id": schema.StringAttribute{
				MarkdownDescription: "ID of the resource being tagged, for example a monitor ID. " +
					"Changing it replaces the mapping.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 128)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (m *costAllocationTagModel) toUpsertBody() *costTagUpsertBody {
	return &costTagUpsertBody{Mappings: []costTagMapping{{
		TagKey:       m.TagKey.ValueString(),
		TagValue:     m.TagValue.ValueString(),
		ResourceType: m.ResourceType.ValueString(),
		ResourceID:   m.ResourceID.ValueString(),
	}}}
}

// findCostTagRow picks this mapping out of a listing. The key is compared
// case-insensitively because the API lower-cases it on write, so a config that
// says "Team" still finds the stored "team" row.
func findCostTagRow(rows []costTagRow, tagKey, resourceType, resourceID string) *costTagRow {
	for i := range rows {
		if strings.EqualFold(strings.TrimSpace(rows[i].TagKey), strings.TrimSpace(tagKey)) &&
			rows[i].ResourceType == resourceType &&
			rows[i].ResourceID == resourceID {
			return &rows[i]
		}
	}
	return nil
}

// applyRow folds a listing row into state.
//
// tag_key and resource_id keep the CONFIGURED spelling rather than the stored
// one: the API lower-cases the key and trims the id, and writing the normalised
// form back into a Required attribute fails the apply with "was
// cty.StringVal(\"Team\"), but now cty.StringVal(\"team\")". tag_value takes the
// server value whenever it differs by more than that trim, because a value
// someone changed in the portal is real drift this resource should plan away.
func (m *costAllocationTagModel) applyRow(row *costTagRow) {
	m.ID = types.StringValue(row.ID)
	if strings.TrimSpace(m.TagValue.ValueString()) != row.TagValue {
		m.TagValue = types.StringValue(row.TagValue)
	}
}

// fetchRow lists the tenant's mappings for this key and category and returns
// ours, or nil when it is gone. Bounded by the API's own take cap; the filters
// keep the listing to one key rather than the whole tenant.
func (r *costAllocationTagResource) fetchRow(ctx context.Context, m *costAllocationTagModel) (*costTagRow, error) {
	query := url.Values{}
	query.Set("key", m.TagKey.ValueString())
	query.Set("resourceType", m.ResourceType.ValueString())
	query.Set("take", "5000")

	var list costTagListResponse
	if err := r.client.Get(ctx, r.client.Tenantf("/billing", "/cost-tags"), query, &list); err != nil {
		return nil, err
	}
	return findCostTagRow(list.Items, m.TagKey.ValueString(),
		m.ResourceType.ValueString(), m.ResourceID.ValueString()), nil
}

// upsert writes the mapping. The API drops a mapping that fails its own length
// rules and reports how many it wrote, so a zero count is a silent rejection
// rather than a success.
func (r *costAllocationTagResource) upsert(ctx context.Context, m *costAllocationTagModel) error {
	var written costTagUpsertResponse
	if err := r.client.Put(ctx, r.client.Tenantf("/billing", "/cost-tags"), m.toUpsertBody(), &written); err != nil {
		return err
	}
	if written.Written == 0 {
		return fmt.Errorf("the billing API accepted the request but wrote no mapping; "+
			"check that tag_key (1-64 chars), tag_value (1-128) and resource_id (1-128) are within range, "+
			"got key %q value %q resource %q",
			m.TagKey.ValueString(), m.TagValue.ValueString(), m.ResourceID.ValueString())
	}
	return nil
}

func (r *costAllocationTagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan costAllocationTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.upsert(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error creating cost allocation tag", err.Error())
		return
	}

	// The upsert returns a count, not the row, so read the id back.
	row, err := r.fetchRow(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error reading back cost allocation tag", err.Error())
		return
	}
	if row == nil {
		resp.Diagnostics.AddError("Error reading back cost allocation tag",
			"The mapping was written but is not in the tenant's cost-tag listing.")
		return
	}

	plan.applyRow(row)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *costAllocationTagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state costAllocationTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	row, err := r.fetchRow(ctx, &state)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading cost allocation tag", err.Error())
		return
	}
	if row == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.applyRow(row)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only ever runs for a tag_value change; the identifying attributes force
// replacement. The same upsert updates the existing row, so the id is unchanged.
func (r *costAllocationTagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan costAllocationTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.upsert(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error updating cost allocation tag", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *costAllocationTagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state costAllocationTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/billing", "/cost-tags/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting cost allocation tag", err.Error())
	}
}

// ImportState takes "<tag_key>/<resource_type>/<resource_id>" rather than the
// mapping id, because the listing the provider reads is addressed by those three
// and not by id. Read fills in id and tag_value.
func (r *costAllocationTagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected import ID in the form \"<tag_key>/<resource_type>/<resource_id>\", got: "+req.ID,
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tag_key"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource_type"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource_id"), parts[2])...)
}
