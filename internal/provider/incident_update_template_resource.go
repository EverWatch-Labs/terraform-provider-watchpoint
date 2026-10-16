package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A reusable incident update body (Status, /incident-update-templates). PUT is a
// full replace; there is no GET by id, so Read lists and selects.

var (
	_ resource.Resource                = (*incidentUpdateTemplateResource)(nil)
	_ resource.ResourceWithConfigure   = (*incidentUpdateTemplateResource)(nil)
	_ resource.ResourceWithImportState = (*incidentUpdateTemplateResource)(nil)
)

// NewIncidentUpdateTemplateResource is the resource constructor wired into the provider.
func NewIncidentUpdateTemplateResource() resource.Resource { return &incidentUpdateTemplateResource{} }

type incidentUpdateTemplateResource struct {
	client *client.Client
}

type incidentUpdateTemplateModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	BodyMarkdown types.String `tfsdk:"body_markdown"`
	Status       types.String `tfsdk:"status"`
}

type incidentUpdateTemplateBody struct {
	Name         string  `json:"name"`
	BodyMarkdown string  `json:"bodyMarkdown"`
	Status       *string `json:"status,omitempty"`
}

type incidentUpdateTemplateResponse struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Status       *string `json:"status"`
	BodyMarkdown string  `json:"bodyMarkdown"`
}

func (r *incidentUpdateTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_update_template"
}

func (r *incidentUpdateTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *incidentUpdateTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A reusable incident update for the status page composer. The body may use " +
			"`{{component}}`, `{{start_time}}`, `{{title}}`, `{{status}}`, `{{impact}}`, `{{page}}` and " +
			"`{{duration}}`, filled from the incident when the template is inserted; unknown variables are " +
			"left as written. Requires `status_pages.manage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Template ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Unique name within the tenant (120 characters max).",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 120)},
			},
			"body_markdown": schema.StringAttribute{
				MarkdownDescription: "Markdown body (10000 characters max).",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 10000)},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Incident status the composer switches to when the template is used: " +
					"`Investigating`, `Identified`, `Monitoring` or `Resolved`.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.OneOf("Investigating", "Identified", "Monitoring", "Resolved")},
			},
		},
	}
}

func (m *incidentUpdateTemplateModel) toBody() *incidentUpdateTemplateBody {
	return &incidentUpdateTemplateBody{
		Name:         m.Name.ValueString(),
		BodyMarkdown: m.BodyMarkdown.ValueString(),
		Status:       strPtr(m.Status),
	}
}

func (m *incidentUpdateTemplateModel) applyResponse(in *incidentUpdateTemplateResponse) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.BodyMarkdown = types.StringValue(in.BodyMarkdown)
	m.Status = strFromPtr(in.Status)
}

func (r *incidentUpdateTemplateResource) path(id string) string {
	if id == "" {
		return r.client.Tenantf("/status", "/incident-update-templates")
	}
	return r.client.Tenantf("/status", "/incident-update-templates/%s", id)
}

func (r *incidentUpdateTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan incidentUpdateTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got incidentUpdateTemplateResponse
	if err := r.client.Post(ctx, r.path(""), plan.toBody(), &got); err != nil {
		resp.Diagnostics.AddError("Error creating incident update template", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *incidentUpdateTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state incidentUpdateTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var list struct {
		Templates []incidentUpdateTemplateResponse `json:"templates"`
	}
	if err := r.client.Get(ctx, r.path(""), nil, &list); err != nil {
		resp.Diagnostics.AddError("Error reading incident update template", err.Error())
		return
	}
	for i := range list.Templates {
		if list.Templates[i].ID == state.ID.ValueString() {
			state.applyResponse(&list.Templates[i])
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *incidentUpdateTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan incidentUpdateTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got incidentUpdateTemplateResponse
	if err := r.client.Put(ctx, r.path(plan.ID.ValueString()), plan.toBody(), &got); err != nil {
		resp.Diagnostics.AddError("Error updating incident update template", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *incidentUpdateTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state incidentUpdateTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path(state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting incident update template", err.Error())
	}
}

func (r *incidentUpdateTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
