package provider

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// annotations lists the chart annotations (deploy markers, config changes,
// hand-placed notes) in a time window (Monitoring, /annotations). Read-only: a
// marker is an event that happened, not configuration to reconcile, so it is a
// data source. CI pipelines post them through the API's /annotations/ci route.

var (
	_ datasource.DataSource              = (*annotationsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*annotationsDataSource)(nil)
)

// NewAnnotationsDataSource is the data source constructor wired into the provider.
func NewAnnotationsDataSource() datasource.DataSource { return &annotationsDataSource{} }

type annotationsDataSource struct {
	client *client.Client
}

type annotationModel struct {
	ID            types.String `tfsdk:"id"`
	Label         types.String `tfsdk:"label"`
	Description   types.String `tfsdk:"description"`
	OccurredAt    types.String `tfsdk:"occurred_at"`
	DurationMs    types.Int64  `tfsdk:"duration_ms"`
	Source        types.String `tfsdk:"source"`
	Color         types.String `tfsdk:"color"`
	RefKind       types.String `tfsdk:"ref_kind"`
	RefID         types.String `tfsdk:"ref_id"`
	Tags          types.Set    `tfsdk:"tags"`
	CreatedByName types.String `tfsdk:"created_by_name"`
}

type annotationsDataSourceModel struct {
	From            types.String      `tfsdk:"from"`
	To              types.String      `tfsdk:"to"`
	Source          types.String      `tfsdk:"source"`
	RefKind         types.String      `tfsdk:"ref_kind"`
	RefID           types.String      `tfsdk:"ref_id"`
	IncludeUnscoped types.Bool        `tfsdk:"include_unscoped"`
	Limit           types.Int64       `tfsdk:"limit"`
	Annotations     []annotationModel `tfsdk:"annotations"`
}

type annotationRow struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	Description   *string `json:"description"`
	OccurredAt    string  `json:"occurredAt"`
	DurationMs    *int64  `json:"durationMs"`
	Source        string  `json:"source"`
	Color         *string `json:"color"`
	RefKind       *string `json:"refKind"`
	RefID         *string `json:"refId"`
	Tags          *string `json:"tags"`
	CreatedByName *string `json:"createdByName"`
}

func (d *annotationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_annotations"
}

func (d *annotationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	} else {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
	}
}

func (d *annotationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists chart annotations (deploy and config markers, hand-placed notes) in a time " +
			"window, oldest first. Without `from` / `to` the window is the last 7 days, so the result moves " +
			"as time passes; pin the window when the list feeds other resources.",
		Attributes: map[string]schema.Attribute{
			"from": schema.StringAttribute{
				MarkdownDescription: "Window start (RFC 3339, UTC). Defaults to 7 days before now.",
				Optional:            true,
			},
			"to": schema.StringAttribute{
				MarkdownDescription: "Window end (RFC 3339, UTC). Defaults to now.",
				Optional:            true,
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "Only annotations from this source, for example `manual`, `deploy` or `config`.",
				Optional:            true,
			},
			"ref_kind": schema.StringAttribute{
				MarkdownDescription: "Only annotations scoped to this kind of resource: `monitor` or `service`.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.OneOf("monitor", "service")},
			},
			"ref_id": schema.StringAttribute{
				MarkdownDescription: "Only annotations scoped to this monitor id or service name.",
				Optional:            true,
			},
			"include_unscoped": schema.BoolAttribute{
				MarkdownDescription: "With `ref_kind` and `ref_id`, also return tenant-wide annotations (what a " +
					"chart for that resource shows), but never ones scoped to another resource.",
				Optional: true,
			},
			"limit": schema.Int64Attribute{
				MarkdownDescription: "Maximum rows returned, `1` to `1000`. Defaults to `500`.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 1000)},
			},
			"annotations": schema.ListNestedAttribute{
				MarkdownDescription: "Annotations in the window, oldest first.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          computed("Annotation ID."),
						"label":       computed("Short label shown on the chart."),
						"description": computed("Longer description, if any."),
						"occurred_at": computed("When it happened (RFC 3339)."),
						"duration_ms": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Length of a range annotation in milliseconds; null for a point.",
						},
						"source":   computed("Where it came from (`manual`, `deploy`, `config`, ...)."),
						"color":    computed("Display color, if set."),
						"ref_kind": computed("`monitor` or `service` when scoped; null when tenant-wide."),
						"ref_id":   computed("Scoped monitor id or service name."),
						"tags": schema.SetAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Tags on the annotation. Unordered.",
						},
						"created_by_name": computed("Name or email of the author, for hand-placed annotations."),
					},
				},
			},
		},
	}
}

func (d *annotationsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config annotationsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query := url.Values{}
	for key, v := range map[string]types.String{
		"from": config.From, "to": config.To, "source": config.Source, "refKind": config.RefKind, "refId": config.RefID,
	} {
		if s := strPtr(v); s != nil {
			query.Set(key, *s)
		}
	}
	if config.IncludeUnscoped.ValueBool() {
		query.Set("includeUnscoped", "true")
	}
	if l := int64Ptr(config.Limit); l != nil {
		query.Set("limit", strconv.FormatInt(*l, 10))
	}

	var rows []annotationRow
	if err := d.client.Get(ctx, d.client.Tenantf("/monitoring", "/annotations"), query, &rows); err != nil {
		resp.Diagnostics.AddError("Error reading annotations", err.Error())
		return
	}

	config.Annotations = make([]annotationModel, 0, len(rows))
	for i := range rows {
		a := &rows[i]
		config.Annotations = append(config.Annotations, annotationModel{
			ID:            types.StringValue(a.ID),
			Label:         types.StringValue(a.Label),
			Description:   strFromPtr(a.Description),
			OccurredAt:    types.StringValue(a.OccurredAt),
			DurationMs:    int64FromPtr(a.DurationMs),
			Source:        types.StringValue(a.Source),
			Color:         strFromPtr(a.Color),
			RefKind:       strFromPtr(a.RefKind),
			RefID:         strFromPtr(a.RefID),
			Tags:          setFromCSVPtr(ctx, a.Tags, &resp.Diagnostics),
			CreatedByName: strFromPtr(a.CreatedByName),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
