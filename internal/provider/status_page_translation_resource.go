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

// Per-language overrides of status page text (Status, /pages/{id}/translations).
// Only the keys in config are owned; translations added in the portal are left alone.

var (
	_ resource.Resource                = (*statusPageTranslationResource)(nil)
	_ resource.ResourceWithConfigure   = (*statusPageTranslationResource)(nil)
	_ resource.ResourceWithImportState = (*statusPageTranslationResource)(nil)
)

// NewStatusPageTranslationResource is the resource constructor wired into the provider.
func NewStatusPageTranslationResource() resource.Resource { return &statusPageTranslationResource{} }

type statusPageTranslationResource struct {
	client *client.Client
}

type statusPageTranslationModel struct {
	ID           types.String       `tfsdk:"id"`
	StatusPageID types.String       `tfsdk:"status_page_id"`
	Language     types.String       `tfsdk:"language"`
	Entries      []translationEntry `tfsdk:"entries"`
}

type translationEntry struct {
	EntityType string `tfsdk:"entity_type" json:"entityType"`
	EntityID   string `tfsdk:"entity_id" json:"entityId"`
	Field      string `tfsdk:"field" json:"field"`
	Value      string `tfsdk:"value" json:"value"`
}

func (e translationEntry) key() string { return e.EntityType + "/" + e.EntityID + "/" + e.Field }

type translationsBody struct {
	Language string             `json:"language"`
	Entries  []translationEntry `json:"entries"`
}

type translationsResponse struct {
	Entries []translationEntry `json:"entries"`
}

func (r *statusPageTranslationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_translation"
}

func (r *statusPageTranslationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *statusPageTranslationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Translations of a status page's own text into one language. Visitors who pick that " +
			"language see these values in place of the authored text. Only the entries listed here are managed, so " +
			"translations added in the portal (for example of incident updates) are left alone. Pair it with " +
			"`enabled_languages` on `watchpoint_status_page`. Requires the `status_page_localization` feature and " +
			"`status_pages.manage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "`<status_page_id>/<language>`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status_page_id": schema.StringAttribute{
				MarkdownDescription: "The status page.",
				Required:            true,
				PlanModifiers:       replace,
			},
			"language": schema.StringAttribute{
				MarkdownDescription: "Target language: `" + strings.Join(statusPageLanguages, "`, `") + "`.",
				Required:            true,
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.OneOf(statusPageLanguages...)},
			},
			"entries": schema.SetNestedAttribute{
				MarkdownDescription: "Translated values, one per entity field.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"entity_type": schema.StringAttribute{
						MarkdownDescription: "`page`, `component`, `group`, `incident`, `incident_update` or `maintenance`.",
						Required:            true,
						Validators: []validator.String{stringvalidator.OneOf(
							"page", "component", "group", "incident", "incident_update", "maintenance")},
					},
					"entity_id": schema.StringAttribute{
						MarkdownDescription: "ID of the entity; it must belong to this status page.",
						Required:            true,
					},
					"field": schema.StringAttribute{
						MarkdownDescription: "`name`, `description`, `footer_text` (page), `title` (incident, maintenance) " +
							"or `body` (incident_update).",
						Required: true,
						Validators: []validator.String{stringvalidator.OneOf(
							"name", "description", "footer_text", "title", "body")},
					},
					"value": schema.StringAttribute{
						MarkdownDescription: "The translated text.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
				}},
			},
		},
	}
}

func (r *statusPageTranslationResource) path(pageID string) string {
	return r.client.Tenantf("/status", "/pages/%s/translations", pageID)
}

// put writes want and blanks every key in drop that want no longer has (an empty value deletes).
func (r *statusPageTranslationResource) put(ctx context.Context, m *statusPageTranslationModel, drop []translationEntry) error {
	keep := map[string]bool{}
	entries := append([]translationEntry{}, m.Entries...)
	for _, e := range m.Entries {
		keep[e.key()] = true
	}
	for _, e := range drop {
		if !keep[e.key()] {
			e.Value = ""
			entries = append(entries, e)
		}
	}
	return r.client.Put(ctx, r.path(m.StatusPageID.ValueString()),
		&translationsBody{Language: m.Language.ValueString(), Entries: entries}, nil)
}

func (r *statusPageTranslationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageTranslationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &plan, nil); err != nil {
		resp.Diagnostics.AddError("Error saving status page translations", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.StatusPageID.ValueString() + "/" + plan.Language.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageTranslationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageTranslationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got translationsResponse
	err := r.client.Get(ctx, r.path(state.StatusPageID.ValueString()), url.Values{"lang": {state.Language.ValueString()}}, &got)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading status page translations", err.Error())
		return
	}
	// A fresh import (no entries yet) adopts everything; otherwise keep only owned keys.
	owned := map[string]bool{}
	for _, e := range state.Entries {
		owned[e.key()] = true
	}
	state.Entries = []translationEntry{}
	for _, e := range got.Entries {
		if len(owned) == 0 || owned[e.key()] {
			state.Entries = append(state.Entries, e)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *statusPageTranslationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state statusPageTranslationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, &plan, state.Entries); err != nil {
		resp.Diagnostics.AddError("Error saving status page translations", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageTranslationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageTranslationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	drop := state.Entries
	state.Entries = nil
	if err := r.put(ctx, &state, drop); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error removing status page translations", err.Error())
	}
}

// ImportState takes `<status_page_id>/<language>` and adopts every existing entry for that language.
func (r *statusPageTranslationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pageID, lang, ok := strings.Cut(req.ID, "/")
	if !ok || pageID == "" || lang == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("expected <status_page_id>/<language>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), pageID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("language"), lang)...)
}
