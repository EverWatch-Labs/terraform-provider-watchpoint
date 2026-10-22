package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Shared pieces for the probed monitor types added after the original seven
// (mail, UDP, domain). Each type file embeds monitorCommonModel for the fields
// every monitor carries and adds only what is specific to it. The older monitor
// files predate this and keep their own copies.

type monitorCommonModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	CheckIntervalSeconds types.Int64  `tfsdk:"check_interval_seconds"`
	TimeoutSeconds       types.Int64  `tfsdk:"timeout_seconds"`
	LocationProfileID    types.String `tfsdk:"location_profile_id"`
	ThresholdProfileID   types.String `tfsdk:"threshold_profile_id"`
	AlertProfileID       types.String `tfsdk:"alert_profile_id"`
	SlaProfileID         types.String `tfsdk:"sla_profile_id"`
	GroupID              types.String `tfsdk:"group_id"`
	Tags                 types.Set    `tfsdk:"tags"`
	EnvironmentID        types.String `tfsdk:"environment_id"`
	Status               types.String `tfsdk:"status"`
	LastCheckStatus      types.String `tfsdk:"last_check_status"`
	CreatedAt            types.String `tfsdk:"created_at"`
}

// monitorCommonBody is embedded in each type's create/PATCH body. Create leaves
// unset references out; PATCH sends "" for them, the API's clear signal, so a
// reference removed from config is actually removed.
type monitorCommonBody struct {
	Name                 string  `json:"name"`
	CheckIntervalSeconds *int64  `json:"checkIntervalSeconds,omitempty"`
	TimeoutSeconds       *int64  `json:"timeoutSeconds,omitempty"`
	LocationProfileID    *string `json:"locationProfileId,omitempty"`
	ThresholdProfileID   *string `json:"thresholdProfileId,omitempty"`
	AlertProfileID       *string `json:"alertProfileId,omitempty"`
	GroupID              *string `json:"groupId,omitempty"`
	Tags                 *string `json:"tags,omitempty"`
	EnvironmentID        *string `json:"environmentId,omitempty"`
}

type monitorCommonResponse struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Status               string  `json:"status"`
	CheckIntervalSeconds int64   `json:"checkIntervalSeconds"`
	TimeoutSeconds       int64   `json:"timeoutSeconds"`
	LocationProfileID    *string `json:"locationProfileId"`
	ThresholdProfileID   *string `json:"thresholdProfileId"`
	AlertProfileID       *string `json:"alertProfileId"`
	SlaProfileID         *string `json:"slaProfileId"`
	GroupID              *string `json:"groupId"`
	Tags                 *string `json:"tags"`
	EnvironmentID        *string `json:"environmentId"`
	LastCheckStatus      *string `json:"lastCheckStatus"`
	CreatedAt            string  `json:"createdAt"`
}

func (m *monitorCommonModel) body(ctx context.Context, update bool, ds *diagSink) monitorCommonBody {
	b := monitorCommonBody{
		Name:                 m.Name.ValueString(),
		CheckIntervalSeconds: int64Ptr(m.CheckIntervalSeconds),
		TimeoutSeconds:       int64Ptr(m.TimeoutSeconds),
		LocationProfileID:    strPtr(m.LocationProfileID),
		ThresholdProfileID:   strPtr(m.ThresholdProfileID),
		AlertProfileID:       strPtr(m.AlertProfileID),
		GroupID:              strPtr(m.GroupID),
		Tags:                 csvPtrFromSet(ctx, m.Tags, ds.d),
		EnvironmentID:        strPtr(m.EnvironmentID),
	}
	if update {
		b.LocationProfileID = strPtrClearing(m.LocationProfileID)
		b.ThresholdProfileID = strPtrClearing(m.ThresholdProfileID)
		b.AlertProfileID = strPtrClearing(m.AlertProfileID)
		b.GroupID = strPtrClearing(m.GroupID)
		b.Tags = csvPtrClearing(ctx, m.Tags, ds.d)
	}
	return b
}

func (m *monitorCommonModel) apply(ctx context.Context, in *monitorCommonResponse, ds *diagSink) {
	m.ID = types.StringValue(in.ID)
	m.Name = types.StringValue(in.Name)
	m.CheckIntervalSeconds = types.Int64Value(in.CheckIntervalSeconds)
	m.TimeoutSeconds = types.Int64Value(in.TimeoutSeconds)
	m.LocationProfileID = strFromPtr(in.LocationProfileID)
	m.ThresholdProfileID = strFromPtr(in.ThresholdProfileID)
	m.AlertProfileID = strFromPtr(in.AlertProfileID)
	m.SlaProfileID = strFromPtr(in.SlaProfileID)
	m.GroupID = strFromPtr(in.GroupID)
	m.Tags = setFromCSVPtr(ctx, in.Tags, ds.d)
	m.EnvironmentID = strFromPtr(in.EnvironmentID)
	m.Status = types.StringValue(in.Status)
	m.LastCheckStatus = strFromPtr(in.LastCheckStatus)
	m.CreatedAt = types.StringValue(in.CreatedAt)
}

// monitorCommonAttributes returns the shared schema; the caller adds its own
// attributes to the map.
func monitorCommonAttributes(defaultInterval, defaultTimeout int64) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "Monitor ID.",
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Human-readable monitor name; shown across the UI and alerts.",
			Required:            true,
		},
		"check_interval_seconds": schema.Int64Attribute{
			MarkdownDescription: "Seconds between checks. Plans cap the minimum.",
			Optional:            true,
			Computed:            true,
			Default:             int64default.StaticInt64(defaultInterval),
		},
		"timeout_seconds": schema.Int64Attribute{
			MarkdownDescription: "Check timeout in seconds.",
			Optional:            true,
			Computed:            true,
			Default:             int64default.StaticInt64(defaultTimeout),
		},
		"location_profile_id": schema.StringAttribute{
			MarkdownDescription: "Profile that controls which regions run the check. Hosted regions only: " +
				"private locations refuse this monitor type.",
			Optional: true,
		},
		"threshold_profile_id": schema.StringAttribute{
			MarkdownDescription: "Profile that controls consecutive-failure logic.",
			Optional:            true,
		},
		"alert_profile_id": schema.StringAttribute{
			MarkdownDescription: "Profile that controls notification channels and routing.",
			Optional:            true,
		},
		"sla_profile_id": schema.StringAttribute{
			MarkdownDescription: "SLA profile attached to this monitor. Requires the SLA monitoring feature. " +
				"When omitted the monitor keeps whatever it has, which on create is the tenant's default SLA " +
				"profile if one is set.",
			Optional:      true,
			Computed:      true,
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"group_id": schema.StringAttribute{
			MarkdownDescription: "Monitor group this monitor belongs to.",
			Optional:            true,
		},
		"environment_id": environmentIDAttribute(),
		"tags": schema.SetAttribute{
			MarkdownDescription: "Free-form tags surfaced in filters and SGL queries. Unordered.",
			Optional:            true,
			ElementType:         types.StringType,
		},
		"status": schema.StringAttribute{
			MarkdownDescription: "Current monitor status (e.g. `Active`, `Paused`).",
			Computed:            true,
		},
		"last_check_status": schema.StringAttribute{
			MarkdownDescription: "Status of the last check (`Success`, `Warning`, `Critical`, ...), or null.",
			Computed:            true,
		},
		"created_at": schema.StringAttribute{
			MarkdownDescription: "Creation timestamp (RFC 3339).",
			Computed:            true,
		},
	}
}

// environmentIDAttribute is the environment_id schema every monitor type shares.
func environmentIDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "Environment the monitor belongs to (a `watchpoint_environment` id). When omitted, a new " +
			"monitor joins the tenant's default environment and an existing one keeps its environment. Reads return " +
			"the effective environment, so a monitor that follows the default shows the default's id.",
		Optional:      true,
		Computed:      true,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

// contentChangeModeAttribute and contentIgnoreRegexAttribute are the HTTP and synthetic content change fields.
func contentChangeModeAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "Content change detection: `off`, `record` (store a snapshot when the normalized body " +
			"changes) or `alert` (also raise a Warning until the change is accepted in the portal). Defaults to " +
			"`off`. Turning it on requires the content change detection feature.",
		Optional:   true,
		Computed:   true,
		Default:    stringdefault.StaticString("off"),
		Validators: []validator.String{stringvalidator.OneOf("off", "record", "alert")},
	}
}

func contentIgnoreRegexAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "Regular expression whose matches are removed before content is compared, for " +
			"timestamps, nonces and similar noise. RE2 subset: no backreferences or lookaround.",
		Optional:   true,
		Validators: []validator.String{stringvalidator.LengthAtMost(1024)},
	}
}

// monitorAPI is the create/read/update/delete flow every per-type monitor
// endpoint shares: POST returns {id}, the SLA profile goes through its own
// attach endpoint, and every write is followed by a GET of the canonical row.
type monitorAPI struct {
	client *client.Client
	kind   string // path segment under /monitors, e.g. "mail"
}

func (a monitorAPI) path(id string) string {
	if id == "" {
		return a.client.Tenantf("/monitoring", "/monitors/%s", a.kind)
	}
	return a.client.Tenantf("/monitoring", "/monitors/%s/%s", a.kind, id)
}

func (a monitorAPI) create(ctx context.Context, body any, sla types.String, out any) error {
	var created idNameResponse
	if err := a.client.PostManaged(ctx, a.path(""), body, &created, monitorAddress(a.kind)); err != nil {
		return err
	}
	if slaIsSet(sla) {
		if err := applySlaProfile(ctx, a.client, monitorAddress(a.kind), created.ID, sla); err != nil {
			return err
		}
	}
	return a.client.Get(ctx, a.path(created.ID), nil, out)
}

func (a monitorAPI) update(ctx context.Context, id string, body any, sla, priorSla types.String, out any) error {
	if err := a.client.PatchManaged(ctx, a.path(id), body, nil, monitorAddress(a.kind)); err != nil {
		return err
	}
	if !sla.Equal(priorSla) {
		if err := applySlaProfile(ctx, a.client, monitorAddress(a.kind), id, sla); err != nil {
			return err
		}
	}
	return a.client.Get(ctx, a.path(id), nil, out)
}

// read returns found=false on a 404 so the caller can drop the resource.
func (a monitorAPI) read(ctx context.Context, id string, out any) (bool, error) {
	if err := a.client.Get(ctx, a.path(id), nil, out); err != nil {
		if client.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a monitorAPI) delete(ctx context.Context, id string) error {
	if err := a.client.DeleteManaged(ctx, a.path(id), monitorAddress(a.kind)); err != nil && !client.IsNotFound(err) {
		return err
	}
	return nil
}

// monitorAddress is the resource type name sent as the managed-by address.
func monitorAddress(kind string) string { return "watchpoint_" + kind + "_monitor" }

// importManagedMonitor imports by id, then sends an empty managed PATCH so the
// server records the monitor as managed by Terraform. Failure only warns: the next apply stamps it.
func importManagedMonitor(ctx context.Context, c *client.Client, kind string, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
	if resp.Diagnostics.HasError() || c == nil {
		return
	}
	err := c.PatchManaged(ctx, c.Tenantf("/monitoring", "/monitors/%s/%s", kind, req.ID), struct{}{}, nil, monitorAddress(kind))
	if err != nil {
		resp.Diagnostics.AddWarning("Monitor not yet marked as managed by Terraform",
			"The import succeeded, but marking the monitor as managed failed: "+err.Error()+
				". The next terraform apply that changes this monitor records it.")
	}
}
