package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An admin-added status page subscriber that is not a person: a signed webhook,
// or a Slack / Microsoft Teams incoming webhook. Admin-added subscribers are
// confirmed immediately. There is no PATCH, so every input replaces. Chat URLs
// come back masked and the webhook signing secret only once, so both are kept
// from the plan.

var (
	_ resource.Resource              = (*statusPageSubscriberResource)(nil)
	_ resource.ResourceWithConfigure = (*statusPageSubscriberResource)(nil)
)

// No import: the URL is masked on read and the signing secret is shown once, so
// an imported subscriber could only ever be replaced.

// NewStatusPageSubscriberResource is the resource constructor wired into the provider.
func NewStatusPageSubscriberResource() resource.Resource { return &statusPageSubscriberResource{} }

type statusPageSubscriberResource struct {
	client *client.Client
}

type statusPageSubscriberModel struct {
	ID            types.String `tfsdk:"id"`
	StatusPageID  types.String `tfsdk:"status_page_id"`
	Channel       types.String `tfsdk:"channel"`
	URL           types.String `tfsdk:"url"`
	ComponentIDs  types.Set    `tfsdk:"component_ids"`
	SigningSecret types.String `tfsdk:"signing_secret"`
	Status        types.String `tfsdk:"status"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

type subscriberResponse struct {
	ID                 string   `json:"id"`
	Channel            string   `json:"channel"`
	Status             string   `json:"status"`
	CreatedAt          string   `json:"createdAt"`
	ComponentFilterIDs []string `json:"componentFilterIds"`
}

func (r *statusPageSubscriberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_subscriber"
}

func (r *statusPageSubscriberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *statusPageSubscriberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A status page subscriber that is a system rather than a person: a signed " +
			"webhook, or a Slack or Microsoft Teams channel. Incident and maintenance updates are delivered " +
			"to it. Admin-added subscribers are confirmed immediately. Every attribute replaces the " +
			"subscriber when changed. Requires the `status_pages` and `incident_management` features.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Subscriber ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status_page_id": schema.StringAttribute{
				MarkdownDescription: "Status page to subscribe to.",
				Required:            true,
				PlanModifiers:       replace,
			},
			"channel": schema.StringAttribute{
				MarkdownDescription: "`Webhook`, `Slack` or `Teams`.",
				Required:            true,
				PlanModifiers:       replace,
				Validators:          []validator.String{stringvalidator.OneOf("Webhook", "Slack", "Teams")},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "HTTPS endpoint. Slack URLs must be on `hooks.slack.com`; Teams URLs on " +
					"`*.webhook.office.com`, `*.logic.azure.com` or `*.powerplatform.com`. Chat URLs carry a " +
					"token and are returned masked, so the configured value is kept in state.",
				Required:      true,
				Sensitive:     true,
				PlanModifiers: replace,
			},
			"component_ids": schema.SetAttribute{
				MarkdownDescription: "Only notify for these components (Slack and Teams only). Omit for every component.",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"signing_secret": schema.StringAttribute{
				MarkdownDescription: "HMAC secret for verifying webhook deliveries (`Webhook` only). Returned " +
					"once at creation and not recoverable by import.",
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Subscriber status, `Confirmed` for an admin-added subscriber.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (m *statusPageSubscriberModel) applyResponse(ctx context.Context, in *subscriberResponse, ds *diagSink) {
	m.ID = types.StringValue(in.ID)
	m.Channel = types.StringValue(in.Channel)
	m.Status = types.StringValue(in.Status)
	m.CreatedAt = types.StringValue(in.CreatedAt)
	m.ComponentIDs = setOrNull(ctx, in.ComponentFilterIDs, ds)
	if m.SigningSecret.IsUnknown() {
		m.SigningSecret = types.StringNull()
	}
}

func (r *statusPageSubscriberResource) base(pageID string) string {
	return r.client.Tenantf("/status", "/pages/%s/subscribers", pageID)
}

func (r *statusPageSubscriberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageSubscriberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ds := &diagSink{d: &resp.Diagnostics}
	base := r.base(plan.StatusPageID.ValueString())
	var sub subscriberResponse
	if plan.Channel.ValueString() == "Webhook" {
		var out struct {
			Subscriber    subscriberResponse `json:"subscriber"`
			SigningSecret string             `json:"signingSecret"`
		}
		if err := r.client.Post(ctx, base+"/webhooks", map[string]string{"url": plan.URL.ValueString()}, &out); err != nil {
			resp.Diagnostics.AddError("Error creating status page subscriber", err.Error())
			return
		}
		sub = out.Subscriber
		plan.SigningSecret = types.StringValue(out.SigningSecret)
	} else {
		body := map[string]any{
			"channel":      plan.Channel.ValueString(),
			"url":          plan.URL.ValueString(),
			"componentIds": stringSliceFromSet(ctx, plan.ComponentIDs, ds.d),
		}
		if err := r.client.Post(ctx, base+"/chat", body, &sub); err != nil {
			resp.Diagnostics.AddError("Error creating status page subscriber", err.Error())
			return
		}
	}
	plan.applyResponse(ctx, &sub, ds)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageSubscriberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageSubscriberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var rows []subscriberResponse
	if err := r.client.Get(ctx, r.base(state.StatusPageID.ValueString()), nil, &rows); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading status page subscriber", err.Error())
		return
	}
	for i := range rows {
		if rows[i].ID == state.ID.ValueString() {
			state.applyResponse(ctx, &rows[i], &diagSink{d: &resp.Diagnostics})
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

// Update is unreachable: every input attribute requires replacement.
func (r *statusPageSubscriberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageSubscriberModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *statusPageSubscriberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageSubscriberModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	path := r.base(state.StatusPageID.ValueString()) + "/" + state.ID.ValueString()
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting status page subscriber", err.Error())
	}
}
