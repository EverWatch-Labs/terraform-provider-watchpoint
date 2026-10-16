package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An event subscription on the unified event bus (Identity,
// /event-subscriptions): signed HTTPS deliveries of the chosen event types. The
// signing secret is returned only on create and on rotation, so it lives in state
// (Sensitive) the way watchpoint_api_key keeps its secret. There is no GET by id:
// Read lists and selects.

var (
	_ resource.Resource                = (*eventSubscriptionResource)(nil)
	_ resource.ResourceWithConfigure   = (*eventSubscriptionResource)(nil)
	_ resource.ResourceWithImportState = (*eventSubscriptionResource)(nil)
)

// NewEventSubscriptionResource is the resource constructor wired into the provider.
func NewEventSubscriptionResource() resource.Resource { return &eventSubscriptionResource{} }

type eventSubscriptionResource struct {
	client *client.Client
}

type eventSubscriptionModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	URL           types.String `tfsdk:"url"`
	EventTypes    types.Set    `tfsdk:"event_types"`
	IsActive      types.Bool   `tfsdk:"is_active"`
	RotateTrigger types.String `tfsdk:"rotate_trigger"`
	SigningSecret types.String `tfsdk:"signing_secret"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

type eventSubscriptionBody struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	EventTypes []string `json:"eventTypes"`
	IsActive   *bool    `json:"isActive,omitempty"`
}

type eventSubscriptionResponse struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	IsActive      bool     `json:"isActive"`
	EventTypes    []string `json:"eventTypes"`
	CreatedAt     string   `json:"createdAt"`
	SigningSecret string   `json:"signingSecret"`
}

func (r *eventSubscriptionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_event_subscription"
}

func (r *eventSubscriptionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *eventSubscriptionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A subscription on the unified event bus: every event of the chosen types is " +
			"POSTed, HMAC-signed, to `url`. Requires the `unified_event_bus` feature and `settings.manage`. " +
			"The signing secret is returned only at creation and on rotation, so it is stored in state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Subscription ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name.",
				Required:            true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "HTTPS endpoint the events are delivered to. Private and metadata addresses " +
					"are refused.",
				Required: true,
			},
			"event_types": schema.SetAttribute{
				MarkdownDescription: "Event types to deliver, from the catalog at `GET /identity/event-catalog` " +
					"(for example `alert.triggered`, `incident.opened`). At least one.",
				Required:    true,
				ElementType: types.StringType,
				Validators:  []validator.Set{setvalidator.SizeAtLeast(1)},
			},
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Whether deliveries are sent. The platform turns a subscription off after " +
					"repeated failures; setting this back to `true` re-enables it. Defaults to `true`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"rotate_trigger": schema.StringAttribute{
				MarkdownDescription: "Arbitrary value that rotates `signing_secret` when it changes.",
				Optional:            true,
			},
			"signing_secret": schema.StringAttribute{
				MarkdownDescription: "HMAC signing secret for verifying deliveries. Not recoverable by import.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (m *eventSubscriptionModel) toBody(ctx context.Context, ds *diagSink) *eventSubscriptionBody {
	return &eventSubscriptionBody{
		Name:       m.Name.ValueString(),
		URL:        m.URL.ValueString(),
		EventTypes: stringSliceFromSet(ctx, m.EventTypes, ds.d),
		IsActive:   boolPtr(m.IsActive),
	}
}

// applyResponse leaves signing_secret alone: only create and rotate return it.
func (m *eventSubscriptionModel) applyResponse(ctx context.Context, in *eventSubscriptionResponse, ds *diagSink) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.URL = types.StringValue(in.URL)
	m.EventTypes = setFromStringSlice(ctx, in.EventTypes, ds.d)
	m.IsActive = types.BoolValue(in.IsActive)
	if in.CreatedAt != "" {
		m.CreatedAt = types.StringValue(in.CreatedAt)
	}
}

func (r *eventSubscriptionResource) path(suffix string) string {
	return r.client.Tenantf("/identity", "/event-subscriptions%s", suffix)
}

func (r *eventSubscriptionResource) find(ctx context.Context, id string) (*eventSubscriptionResponse, error) {
	var rows []eventSubscriptionResponse
	if err := r.client.Get(ctx, r.path(""), nil, &rows); err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i], nil
		}
	}
	return nil, nil
}

func (r *eventSubscriptionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan eventSubscriptionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	body := plan.toBody(ctx, ds)
	var created eventSubscriptionResponse
	if err := r.client.Post(ctx, r.path(""), body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating event subscription", err.Error())
		return
	}
	plan.SigningSecret = types.StringValue(created.SigningSecret)
	// Create always starts active; honour is_active = false with a follow-up PATCH.
	if !plan.IsActive.ValueBool() {
		if err := r.client.Patch(ctx, r.path("/"+created.ID), body, nil); err != nil {
			resp.Diagnostics.AddError("Error pausing new event subscription", err.Error())
			return
		}
	}
	got, err := r.find(ctx, created.ID)
	if err != nil || got == nil {
		resp.Diagnostics.AddError("Error reading event subscription after create", errText(err, "not found"))
		return
	}
	plan.applyResponse(ctx, got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *eventSubscriptionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state eventSubscriptionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.find(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading event subscription", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.applyResponse(ctx, got, &diagSink{d: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *eventSubscriptionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state eventSubscriptionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	id := state.ID.ValueString()
	if err := r.client.Patch(ctx, r.path("/"+id), plan.toBody(ctx, ds), nil); err != nil {
		resp.Diagnostics.AddError("Error updating event subscription", err.Error())
		return
	}
	plan.SigningSecret = state.SigningSecret
	if !plan.RotateTrigger.Equal(state.RotateTrigger) {
		var rotated eventSubscriptionResponse
		if err := r.client.Post(ctx, r.path("/"+id+"/rotate-secret"), nil, &rotated); err != nil {
			resp.Diagnostics.AddError("Error rotating event subscription secret", err.Error())
			return
		}
		plan.SigningSecret = types.StringValue(rotated.SigningSecret)
	}
	got, err := r.find(ctx, id)
	if err != nil || got == nil {
		resp.Diagnostics.AddError("Error reading event subscription after update", errText(err, "not found"))
		return
	}
	plan.applyResponse(ctx, got, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *eventSubscriptionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state eventSubscriptionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Delete(ctx, r.path("/"+state.ID.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting event subscription", err.Error())
	}
}

func (r *eventSubscriptionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
