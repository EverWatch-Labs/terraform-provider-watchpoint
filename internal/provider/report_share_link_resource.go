package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A public link to a scheduled report (Analytics, /reports/{id}/share-links).
// Links are immutable: every change replaces the link. The token is returned
// once, on create, so it lives in state. Destroy revokes it.

var (
	_ resource.Resource                = (*reportShareLinkResource)(nil)
	_ resource.ResourceWithConfigure   = (*reportShareLinkResource)(nil)
	_ resource.ResourceWithImportState = (*reportShareLinkResource)(nil)
)

// NewReportShareLinkResource is the resource constructor wired into the provider.
func NewReportShareLinkResource() resource.Resource { return &reportShareLinkResource{} }

type reportShareLinkResource struct {
	client *client.Client
}

type reportShareLinkModel struct {
	ID            types.String `tfsdk:"id"`
	ReportID      types.String `tfsdk:"report_id"`
	Label         types.String `tfsdk:"label"`
	ExpiresInDays types.Int64  `tfsdk:"expires_in_days"`
	ExpiresAt     types.String `tfsdk:"expires_at"`
	TokenPrefix   types.String `tfsdk:"token_prefix"`
	Token         types.String `tfsdk:"token"`
	URL           types.String `tfsdk:"url"`
}

type reportShareLinkBody struct {
	Label         string `json:"label"`
	ExpiresInDays *int64 `json:"expiresInDays"`
}

type reportShareLinkJSON struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	TokenPrefix string  `json:"tokenPrefix"`
	ExpiresAt   *string `json:"expiresAt"`
	RevokedAt   *string `json:"revokedAt"`
}

func (r *reportShareLinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_report_share_link"
}

func (r *reportShareLinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *reportShareLinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keepStr := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A public, unauthenticated link to a `watchpoint_scheduled_report`'s latest render (and its " +
			"PDF for SLA reports). Anyone with the URL can view it until it expires or is destroyed, which revokes it. " +
			"Every change replaces the link with a new URL. Requires the public report links feature and " +
			"`reports.manage`; refused when the tenant's compliance policy disallows public pages.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Link ID.",
				Computed:            true,
				PlanModifiers:       keepStr,
			},
			"report_id": schema.StringAttribute{
				MarkdownDescription: "ID of the report to share.",
				Required:            true,
				PlanModifiers:       replaceStr,
			},
			"label": schema.StringAttribute{
				MarkdownDescription: "Label shown in the portal (120 characters max). Defaults to `Public link`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("Public link"),
				PlanModifiers:       replaceStr,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 120)},
			},
			"expires_in_days": schema.Int64Attribute{
				MarkdownDescription: "Lifetime in days: `7`, `30` or `90`. Omit for a link that never expires.",
				Optional:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators:          []validator.Int64{int64validator.OneOf(7, 30, 90)},
			},
			"expires_at": schema.StringAttribute{
				MarkdownDescription: "Expiry timestamp (RFC 3339), or null.",
				Computed:            true,
				PlanModifiers:       keepStr,
			},
			"token_prefix": schema.StringAttribute{
				MarkdownDescription: "First characters of the token, as the portal shows them.",
				Computed:            true,
				PlanModifiers:       keepStr,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "The link token. Returned only on create; an imported link has none.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers:       keepStr,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "The public URL. Sensitive, since it carries the token.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers:       keepStr,
			},
		},
	}
}

func (r *reportShareLinkResource) path(reportID, suffix string) string {
	return r.client.Tenantf("/analytics", "/reports/%s/share-links%s", reportID, suffix)
}

func (m *reportShareLinkModel) apply(in *reportShareLinkJSON) {
	m.ID = types.StringValue(in.ID)
	m.Label = types.StringValue(in.Label)
	m.TokenPrefix = types.StringValue(in.TokenPrefix)
	m.ExpiresAt = strFromPtr(in.ExpiresAt)
}

func (r *reportShareLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan reportShareLinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Link  reportShareLinkJSON `json:"link"`
		Token string              `json:"token"`
		URL   string              `json:"url"`
	}
	body := &reportShareLinkBody{Label: plan.Label.ValueString(), ExpiresInDays: int64Ptr(plan.ExpiresInDays)}
	if err := r.client.Post(ctx, r.path(plan.ReportID.ValueString(), ""), body, &out); err != nil {
		resp.Diagnostics.AddError("Error creating report share link", err.Error())
		return
	}
	plan.apply(&out.Link)
	plan.Token = types.StringValue(out.Token)
	plan.URL = types.StringValue(out.URL)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *reportShareLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state reportShareLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Items []reportShareLinkJSON `json:"items"`
	}
	if err := r.client.Get(ctx, r.path(state.ReportID.ValueString(), ""), nil, &out); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading report share link", err.Error())
		return
	}
	for i := range out.Items {
		l := &out.Items[i]
		if l.ID != state.ID.ValueString() {
			continue
		}
		// A link revoked in the portal is gone for good; let Terraform recreate it.
		if l.RevokedAt != nil {
			break
		}
		state.apply(l)
		if state.Token.IsUnknown() {
			state.Token = types.StringNull()
		}
		if state.URL.IsUnknown() {
			state.URL = types.StringNull()
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	resp.State.RemoveResource(ctx)
}

func (r *reportShareLinkResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Report share links are immutable", "Every attribute forces replacement; this is a provider bug.")
}

func (r *reportShareLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state reportShareLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Delete(ctx, r.path(state.ReportID.ValueString(), "/"+state.ID.ValueString()))
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error revoking report share link", err.Error())
	}
}

// ImportState takes "<report_id>/<link_id>".
func (r *reportShareLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	reportID, linkID, ok := strings.Cut(req.ID, "/")
	if !ok || reportID == "" || linkID == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <report_id>/<link_id>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("report_id"), reportID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), linkID)...)
}
