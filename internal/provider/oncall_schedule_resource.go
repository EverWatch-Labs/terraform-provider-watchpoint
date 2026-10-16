package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// An on-call schedule rotates a list of participants on a daily, weekly or
// custom cadence, read in the schedule's IANA time zone. The schedule itself is
// the base layer; extra layers (each with its own rotation, restrictions and
// people) sit above it, and the highest-priority layer that is active and
// staffed wins. POST returns only {id,name}; GET-by-id returns the full schedule,
// so Create re-GETs to populate state. PATCH replaces participants, restrictions
// and layers wholesale, mirroring the alerting API. Ad-hoc overrides are
// operational, time-bound data managed outside Terraform, so they are not
// modeled here.

var (
	_ resource.Resource                = (*onCallScheduleResource)(nil)
	_ resource.ResourceWithConfigure   = (*onCallScheduleResource)(nil)
	_ resource.ResourceWithImportState = (*onCallScheduleResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*onCallScheduleResource)(nil)
)

// NewOnCallScheduleResource is the resource constructor wired into the provider.
func NewOnCallScheduleResource() resource.Resource { return &onCallScheduleResource{} }

type onCallScheduleResource struct {
	client *client.Client
}

// onCallScheduleModel maps schema attributes <-> Terraform state.
type onCallScheduleModel struct {
	ID                  types.String             `tfsdk:"id"`
	Name                types.String             `tfsdk:"name"`
	Description         types.String             `tfsdk:"description"`
	TimeZone            types.String             `tfsdk:"time_zone"`
	RotationType        types.String             `tfsdk:"rotation_type"`
	RotationLengthDays  types.Int64              `tfsdk:"rotation_length_days"`
	RotationLengthHours types.Int64              `tfsdk:"rotation_length_hours"`
	HandoffHourUTC      types.Int64              `tfsdk:"handoff_hour_utc"`
	AnchorUTC           types.String             `tfsdk:"anchor_utc"`
	IsEnabled           types.Bool               `tfsdk:"is_enabled"`
	Restrictions        []onCallRestrictionModel `tfsdk:"restrictions"`
	Participants        []onCallParticipantModel `tfsdk:"participants"`
	Layers              []onCallLayerModel       `tfsdk:"layers"`
	CreatedAt           types.String             `tfsdk:"created_at"`
	UpdatedAt           types.String             `tfsdk:"updated_at"`
}

type onCallParticipantModel struct {
	UserID     types.String `tfsdk:"user_id"`
	UserEmail  types.String `tfsdk:"user_email"`
	UserName   types.String `tfsdk:"user_name"`
	OrderIndex types.Int64  `tfsdk:"order_index"`
}

type onCallRestrictionModel struct {
	Days        []int64     `tfsdk:"days"`
	StartMinute types.Int64 `tfsdk:"start_minute"`
	EndMinute   types.Int64 `tfsdk:"end_minute"`
}

type onCallLayerModel struct {
	Name                types.String             `tfsdk:"name"`
	Priority            types.Int64              `tfsdk:"priority"`
	RotationType        types.String             `tfsdk:"rotation_type"`
	RotationLengthDays  types.Int64              `tfsdk:"rotation_length_days"`
	RotationLengthHours types.Int64              `tfsdk:"rotation_length_hours"`
	HandoffHour         types.Int64              `tfsdk:"handoff_hour"`
	AnchorDate          types.String             `tfsdk:"anchor_date"`
	Restrictions        []onCallRestrictionModel `tfsdk:"restrictions"`
	Participants        []onCallParticipantModel `tfsdk:"participants"`
}

// --- Wire DTOs (camelCase JSON matching the alerting API) ---

type onCallParticipantBody struct {
	UserID     *string `json:"userId,omitempty"`
	UserEmail  *string `json:"userEmail,omitempty"`
	UserName   *string `json:"userName,omitempty"`
	OrderIndex *int64  `json:"orderIndex,omitempty"`
}

type onCallRestrictionWire struct {
	Days        []int64 `json:"days"`
	StartMinute int64   `json:"startMinute"`
	EndMinute   int64   `json:"endMinute"`
}

type onCallLayerBody struct {
	Name                string                  `json:"name"`
	Priority            *int64                  `json:"priority,omitempty"`
	RotationType        *string                 `json:"rotationType,omitempty"`
	RotationLengthDays  *int64                  `json:"rotationLengthDays,omitempty"`
	RotationLengthHours *int64                  `json:"rotationLengthHours,omitempty"`
	HandoffHour         *int64                  `json:"handoffHour,omitempty"`
	AnchorDate          *string                 `json:"anchorDate,omitempty"`
	Restrictions        []onCallRestrictionWire `json:"restrictions"`
	Participants        []onCallParticipantBody `json:"participants"`
}

// onCallScheduleBody: restrictions and layers are always sent on update (as []
// when unset), because the API reads an omitted collection as "leave unchanged"
// and would keep a layer or window the operator deleted from config.
type onCallScheduleBody struct {
	Name                *string                 `json:"name,omitempty"`
	Description         *string                 `json:"description,omitempty"`
	TimeZoneID          *string                 `json:"timeZoneId,omitempty"`
	RotationType        *string                 `json:"rotationType,omitempty"`
	RotationLengthDays  *int64                  `json:"rotationLengthDays,omitempty"`
	RotationLengthHours *int64                  `json:"rotationLengthHours,omitempty"`
	HandoffHourUTC      *int64                  `json:"handoffHourUtc,omitempty"`
	AnchorUTC           *string                 `json:"anchorUtc,omitempty"`
	IsEnabled           *bool                   `json:"isEnabled,omitempty"`
	Participants        []onCallParticipantBody `json:"participants,omitempty"`
	// No omitempty: nil marshals as null ("leave unchanged", used on create when
	// unset) and an empty slice as [] (clear, used on update).
	Restrictions []onCallRestrictionWire `json:"restrictions"`
	Layers       []onCallLayerBody       `json:"layers"`
}

type onCallParticipantResponse struct {
	ID         string  `json:"id"`
	UserID     string  `json:"userId"`
	UserEmail  string  `json:"userEmail"`
	UserName   *string `json:"userName"`
	OrderIndex int64   `json:"orderIndex"`
}

type onCallLayerResponse struct {
	ID                  string                      `json:"id"`
	Name                string                      `json:"name"`
	Priority            int64                       `json:"priority"`
	RotationType        string                      `json:"rotationType"`
	RotationLengthDays  int64                       `json:"rotationLengthDays"`
	RotationLengthHours *int64                      `json:"rotationLengthHours"`
	HandoffHour         int64                       `json:"handoffHour"`
	AnchorDate          string                      `json:"anchorDate"`
	Restrictions        []onCallRestrictionWire     `json:"restrictions"`
	Participants        []onCallParticipantResponse `json:"participants"`
}

type onCallScheduleResponse struct {
	ID                  string                      `json:"id"`
	Name                string                      `json:"name"`
	Description         *string                     `json:"description"`
	TimeZoneID          string                      `json:"timeZoneId"`
	RotationType        string                      `json:"rotationType"`
	RotationLengthDays  int64                       `json:"rotationLengthDays"`
	RotationLengthHours *int64                      `json:"rotationLengthHours"`
	HandoffHourUTC      int64                       `json:"handoffHourUtc"`
	AnchorUTC           string                      `json:"anchorUtc"`
	IsEnabled           bool                        `json:"isEnabled"`
	CreatedAt           string                      `json:"createdAt"`
	UpdatedAt           string                      `json:"updatedAt"`
	Restrictions        []onCallRestrictionWire     `json:"restrictions"`
	Participants        []onCallParticipantResponse `json:"participants"`
	Layers              []onCallLayerResponse       `json:"layers"`
}

func (r *onCallScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_schedule"
}

func (r *onCallScheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

var onCallRotationTypes = []string{"Daily", "Weekly", "Custom"}

func onCallParticipantsAttribute(required bool) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: "Ordered list of rotation participants.",
		Required:            required,
		Optional:            !required,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"user_id": schema.StringAttribute{
					MarkdownDescription: "Watchpoint user ID for the participant.",
					Optional:            true,
				},
				"user_email": schema.StringAttribute{
					MarkdownDescription: "Participant email address (paged when on call).",
					Optional:            true,
				},
				"user_name": schema.StringAttribute{
					MarkdownDescription: "Display name for the participant.",
					Optional:            true,
				},
				"order_index": schema.Int64Attribute{
					MarkdownDescription: "Position in the rotation (ascending). Server-assigned from list " +
						"order when omitted.",
					Optional: true,
					Computed: true,
				},
			},
		},
	}
}

// onCallRestrictionsAttribute is a set: windows are unordered and the API
// stores them as a jsonb array, so list order would only produce churn.
func onCallRestrictionsAttribute(scope string) schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		MarkdownDescription: "Windows, in the schedule's `time_zone`, during which " + scope + " is on call. " +
			"Outside every window it is skipped (a lower layer or the base takes over). Omit for always on. " +
			"At most 14.",
		Optional:   true,
		Validators: []validator.Set{setvalidator.SizeAtMost(14)},
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"days": schema.SetAttribute{
					MarkdownDescription: "Weekdays the window opens on, `0` (Sunday) to `6` (Saturday).",
					Required:            true,
					ElementType:         types.Int64Type,
					Validators: []validator.Set{
						setvalidator.SizeAtLeast(1),
						setvalidator.ValueInt64sAre(int64validator.Between(0, 6)),
					},
				},
				"start_minute": schema.Int64Attribute{
					MarkdownDescription: "Window start, in minutes after local midnight (`0` to `1439`).",
					Required:            true,
					Validators:          []validator.Int64{int64validator.Between(0, 1439)},
				},
				"end_minute": schema.Int64Attribute{
					MarkdownDescription: "Window end, in minutes after local midnight (`0` to `1440`). An end at " +
						"or before the start wraps past midnight, for overnight cover such as `1320` to `360`.",
					Required:   true,
					Validators: []validator.Int64{int64validator.Between(0, 1440)},
				},
			},
		},
	}
}

func (r *onCallScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An on-call schedule that rotates a list of participants on a daily, weekly or " +
			"custom cadence in an IANA time zone, optionally with time restrictions and extra layers. " +
			"Referenced by escalation policy steps to page whoever is currently on call.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "On-call schedule ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable schedule name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Optional:            true,
			},
			"time_zone": schema.StringAttribute{
				MarkdownDescription: "IANA time zone every wall-clock field (handoff hour, anchor date, " +
					"restrictions) is read in, for example `America/New_York`. Handoffs follow daylight-saving " +
					"changes in that zone. Defaults to `UTC`; an unknown zone is rejected by the API.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString("UTC"),
				Validators: []validator.String{stringvalidator.LengthBetween(1, 64)},
			},
			"rotation_type": schema.StringAttribute{
				MarkdownDescription: "Rotation cadence. One of `Daily`, `Weekly`, `Custom`. Defaults to `Weekly`. " +
					"`Custom` uses `rotation_length_days`, or `rotation_length_hours` when that is set.",
				Optional:   true,
				Computed:   true,
				Default:    stringdefault.StaticString("Weekly"),
				Validators: []validator.String{stringvalidator.OneOf(onCallRotationTypes...)},
			},
			"rotation_length_days": schema.Int64Attribute{
				MarkdownDescription: "Number of calendar days each participant holds the rotation (N weeks is " +
					"7N). Defaults to `7` for weekly rotations and `1` for daily.",
				Optional: true,
				Computed: true,
			},
			"rotation_length_hours": schema.Int64Attribute{
				MarkdownDescription: "Fixed-length shifts in elapsed hours, overriding `rotation_length_days`. " +
					"Unlike days, hours do not stretch across a daylight-saving change.",
				Optional:   true,
				Validators: []validator.Int64{int64validator.Between(1, 24*366)},
			},
			"handoff_hour_utc": schema.Int64Attribute{
				MarkdownDescription: "Hour of day (0-23) at which the rotation hands off. Despite the name this " +
					"is read in `time_zone` (it was UTC before schedules had a zone). Defaults to `0`.",
				Optional:   true,
				Computed:   true,
				Default:    int64default.StaticInt64(0),
				Validators: []validator.Int64{int64validator.Between(0, 23)},
			},
			"anchor_utc": schema.StringAttribute{
				MarkdownDescription: "RFC 3339 timestamp the rotation is anchored to; only the date is used. " +
					"Defaults to the current UTC date when omitted.",
				Optional: true,
				Computed: true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the schedule is enabled. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"restrictions": onCallRestrictionsAttribute("the base rotation"),
			"participants": onCallParticipantsAttribute(true),
			"layers": schema.ListNestedAttribute{
				MarkdownDescription: "Extra layers above the base rotation, highest priority first. At any " +
					"moment the first layer that is inside its restrictions and has participants decides who is " +
					"on call; the base rotation covers the rest. At most 10.",
				Optional:   true,
				Validators: []validator.List{listvalidator.SizeAtMost(10)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Layer name, shown when this layer is the one on call.",
							Required:            true,
							Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
						},
						"priority": schema.Int64Attribute{
							MarkdownDescription: "Higher wins. Defaults to list order (first listed is highest).",
							Optional:            true,
							Computed:            true,
						},
						"rotation_type": schema.StringAttribute{
							MarkdownDescription: "`Daily`, `Weekly` or `Custom`. Defaults to `Weekly`.",
							Optional:            true,
							Computed:            true,
							Validators:          []validator.String{stringvalidator.OneOf(onCallRotationTypes...)},
						},
						"rotation_length_days": schema.Int64Attribute{
							MarkdownDescription: "Calendar days per shift. Defaults from `rotation_type`.",
							Optional:            true,
							Computed:            true,
						},
						"rotation_length_hours": schema.Int64Attribute{
							MarkdownDescription: "Fixed-length shifts in elapsed hours, overriding days.",
							Optional:            true,
							Validators:          []validator.Int64{int64validator.Between(1, 24*366)},
						},
						"handoff_hour": schema.Int64Attribute{
							MarkdownDescription: "Local handoff hour (0-23) in the schedule's `time_zone`. Defaults to `0`.",
							Optional:            true,
							Computed:            true,
							Validators:          []validator.Int64{int64validator.Between(0, 23)},
						},
						"anchor_date": schema.StringAttribute{
							MarkdownDescription: "Local date (`YYYY-MM-DD`) the layer's rotation counts from. " +
								"Defaults to the date the layer is created, then kept across edits.",
							Optional:   true,
							Computed:   true,
							Validators: []validator.String{stringvalidator.LengthBetween(10, 10)},
						},
						"restrictions": onCallRestrictionsAttribute("this layer"),
						"participants": onCallParticipantsAttribute(false),
					},
				},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "Last-update timestamp (RFC 3339).",
				Computed:            true,
			},
		},
	}
}

func participantsBody(in []onCallParticipantModel) []onCallParticipantBody {
	out := make([]onCallParticipantBody, 0, len(in))
	for _, p := range in {
		out = append(out, onCallParticipantBody{
			UserID:     strPtr(p.UserID),
			UserEmail:  strPtr(p.UserEmail),
			UserName:   strPtr(p.UserName),
			OrderIndex: int64Ptr(p.OrderIndex),
		})
	}
	return out
}

func restrictionsBody(in []onCallRestrictionModel) []onCallRestrictionWire {
	out := make([]onCallRestrictionWire, 0, len(in))
	for _, r := range in {
		out = append(out, onCallRestrictionWire{Days: r.Days, StartMinute: r.StartMinute.ValueInt64(), EndMinute: r.EndMinute.ValueInt64()})
	}
	return out
}

// hoursBody: on update a removed rotation_length_hours is sent as 0, the API's
// "back to day-based" signal; null would leave the custom hours in force.
func hoursBody(v types.Int64, update bool) *int64 {
	if p := int64Ptr(v); p != nil || !update {
		return p
	}
	zero := int64(0)
	return &zero
}

func (m *onCallScheduleModel) toBody(update bool) *onCallScheduleBody {
	body := &onCallScheduleBody{
		Name:                strPtr(m.Name),
		Description:         strPtr(m.Description),
		TimeZoneID:          strPtr(m.TimeZone),
		RotationType:        strPtr(m.RotationType),
		RotationLengthDays:  int64Ptr(m.RotationLengthDays),
		RotationLengthHours: hoursBody(m.RotationLengthHours, update),
		HandoffHourUTC:      int64Ptr(m.HandoffHourUTC),
		AnchorUTC:           strPtr(m.AnchorUTC),
		IsEnabled:           boolPtr(m.IsEnabled),
		Participants:        participantsBody(m.Participants),
	}
	if update || m.Restrictions != nil {
		body.Restrictions = restrictionsBody(m.Restrictions)
	}
	if update || m.Layers != nil {
		body.Layers = make([]onCallLayerBody, 0, len(m.Layers))
		for _, l := range m.Layers {
			body.Layers = append(body.Layers, onCallLayerBody{
				Name:                l.Name.ValueString(),
				Priority:            int64Ptr(l.Priority),
				RotationType:        strPtr(l.RotationType),
				RotationLengthDays:  int64Ptr(l.RotationLengthDays),
				RotationLengthHours: int64Ptr(l.RotationLengthHours),
				HandoffHour:         int64Ptr(l.HandoffHour),
				AnchorDate:          strPtr(l.AnchorDate),
				Restrictions:        restrictionsBody(l.Restrictions),
				Participants:        participantsBody(l.Participants),
			})
		}
	}
	return body
}

func participantsFromResponse(in []onCallParticipantResponse) []onCallParticipantModel {
	if len(in) == 0 {
		return nil
	}
	out := make([]onCallParticipantModel, 0, len(in))
	for _, p := range in {
		out = append(out, onCallParticipantModel{
			UserID:     strNullIfEmpty(p.UserID),
			UserEmail:  strNullIfEmpty(p.UserEmail),
			UserName:   strFromPtr(p.UserName),
			OrderIndex: types.Int64Value(p.OrderIndex),
		})
	}
	return out
}

func restrictionsFromResponse(in []onCallRestrictionWire) []onCallRestrictionModel {
	if len(in) == 0 {
		return nil
	}
	out := make([]onCallRestrictionModel, 0, len(in))
	for _, r := range in {
		out = append(out, onCallRestrictionModel{Days: r.Days, StartMinute: types.Int64Value(r.StartMinute), EndMinute: types.Int64Value(r.EndMinute)})
	}
	return out
}

// dateOnly trims the API's midnight DateTime ("2026-10-04T00:00:00Z") to the
// YYYY-MM-DD form the schema takes.
func dateOnly(s string) types.String {
	if len(s) >= 10 {
		return types.StringValue(s[:10])
	}
	return strNullIfEmpty(s)
}

func (m *onCallScheduleModel) applyResponse(resp *onCallScheduleResponse) {
	prior := m.Layers
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.TimeZone = types.StringValue(resp.TimeZoneID)
	m.RotationType = types.StringValue(resp.RotationType)
	m.RotationLengthDays = types.Int64Value(resp.RotationLengthDays)
	m.RotationLengthHours = int64FromPtr(resp.RotationLengthHours)
	m.HandoffHourUTC = types.Int64Value(resp.HandoffHourUTC)
	m.AnchorUTC = types.StringValue(resp.AnchorUTC)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)
	m.Restrictions = restrictionsFromResponse(resp.Restrictions)
	m.Participants = participantsFromResponse(resp.Participants)
	if m.Participants == nil {
		m.Participants = []onCallParticipantModel{}
	}

	if len(resp.Layers) == 0 {
		m.Layers = nil
		return
	}
	// The API returns layers by priority, not list order; follow the prior list
	// by name or explicit non-descending priorities fail the apply as an
	// inconsistent result.
	aligned := alignTo(prior, resp.Layers, func(p onCallLayerModel, l onCallLayerResponse) bool {
		return p.Name.ValueString() == l.Name
	})
	layers := make([]onCallLayerModel, 0, len(aligned))
	for _, l := range aligned {
		layers = append(layers, onCallLayerModel{
			Name:                types.StringValue(l.Name),
			Priority:            types.Int64Value(l.Priority),
			RotationType:        types.StringValue(l.RotationType),
			RotationLengthDays:  types.Int64Value(l.RotationLengthDays),
			RotationLengthHours: int64FromPtr(l.RotationLengthHours),
			HandoffHour:         types.Int64Value(l.HandoffHour),
			AnchorDate:          dateOnly(l.AnchorDate),
			Restrictions:        restrictionsFromResponse(l.Restrictions),
			Participants:        participantsFromResponse(l.Participants),
		})
	}
	m.Layers = layers
}

// ModifyPlan keeps each layer's anchor_date when it is not configured. Any edit
// marks the Optional+Computed date unknown, the PATCH would then omit it, and
// since layers are recreated on every update the API would restart the
// rotation from today, silently changing who is on call. The prior date is
// matched by layer name, not list position, so inserting or reordering layers
// does not hand one layer another's anchor. A renamed or new layer stays
// unknown and the API anchors it to today.
func (r *onCallScheduleResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var plan, state onCallScheduleModel
	// A plan whose layers are not yet known (built from another resource's
	// unknown output) cannot be read into the model; there is nothing to keep.
	if req.Plan.Get(ctx, &plan).HasError() || req.State.Get(ctx, &state).HasError() {
		return
	}
	for i, l := range plan.Layers {
		if !l.AnchorDate.IsUnknown() {
			continue
		}
		for _, s := range state.Layers {
			if s.Name.Equal(l.Name) && !s.AnchorDate.IsNull() {
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx,
					path.Root("layers").AtListIndex(i).AtName("anchor_date"), s.AnchorDate)...)
				break
			}
		}
	}
}

func (r *onCallScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan onCallScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var created idNameResponse
	path := r.client.Tenantf("/alerting", "/oncall-schedules")
	if err := r.client.Post(ctx, path, plan.toBody(false), &created); err != nil {
		resp.Diagnostics.AddError("Error creating on-call schedule", err.Error())
		return
	}

	// POST returns only {id,name}; re-read to populate computed fields.
	var got onCallScheduleResponse
	getPath := r.client.Tenantf("/alerting", "/oncall-schedules/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading on-call schedule after create", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *onCallScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state onCallScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got onCallScheduleResponse
	getPath := r.client.Tenantf("/alerting", "/oncall-schedules/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading on-call schedule", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *onCallScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan onCallScheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// PATCH returns a status message, so re-GET to refresh state.
	path := r.client.Tenantf("/alerting", "/oncall-schedules/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, plan.toBody(true), nil); err != nil {
		resp.Diagnostics.AddError("Error updating on-call schedule", err.Error())
		return
	}

	var got onCallScheduleResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading on-call schedule after update", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *onCallScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state onCallScheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/alerting", "/oncall-schedules/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting on-call schedule", err.Error())
	}
}

func (r *onCallScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
