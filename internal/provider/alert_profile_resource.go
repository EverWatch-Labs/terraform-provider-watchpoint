package provider

import (
	"context"

	"github.com/everwatch-labs/terraform-provider-watchpoint/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Alert profiles control notification routing: cooldown/dedup behaviour plus a
// set of channels (email, webhook, Slack, PagerDuty, OpsGenie, etc.). POST
// returns only {id,name}; GET-by-id returns the full profile + channels, so
// Create re-GETs to populate state.

var (
	_ resource.Resource                = (*alertProfileResource)(nil)
	_ resource.ResourceWithConfigure   = (*alertProfileResource)(nil)
	_ resource.ResourceWithImportState = (*alertProfileResource)(nil)
)

// NewAlertProfileResource is the resource constructor wired into the provider.
func NewAlertProfileResource() resource.Resource { return &alertProfileResource{} }

type alertProfileResource struct {
	client *client.Client
}

// alertProfileModel maps schema attributes <-> Terraform state.
type alertProfileModel struct {
	ID                       types.String               `tfsdk:"id"`
	Name                     types.String               `tfsdk:"name"`
	Description              types.String               `tfsdk:"description"`
	IsDefault                types.Bool                 `tfsdk:"is_default"`
	IsGlobal                 types.Bool                 `tfsdk:"is_global"`
	IsEnabled                types.Bool                 `tfsdk:"is_enabled"`
	CooldownMinutes          types.Int64                `tfsdk:"cooldown_minutes"`
	ConsecutiveFailures      types.Int64                `tfsdk:"consecutive_failures"`
	AlertOnRecovery          types.Bool                 `tfsdk:"alert_on_recovery"`
	DeduplicateAcrossRegions types.Bool                 `tfsdk:"deduplicate_across_regions"`
	EscalationPolicyID       types.String               `tfsdk:"escalation_policy_id"`
	OwnerGroupID             types.String               `tfsdk:"owner_group_id"`
	Channels                 []alertProfileChannelModel `tfsdk:"channels"`
	CreatedAt                types.String               `tfsdk:"created_at"`
	UpdatedAt                types.String               `tfsdk:"updated_at"`
}

type alertProfileChannelModel struct {
	Type                   types.String `tfsdk:"type"`
	IsEnabled              types.Bool   `tfsdk:"is_enabled"`
	Name                   types.String `tfsdk:"name"`
	EmailAddresses         types.String `tfsdk:"email_addresses"`
	WebhookURL             types.String `tfsdk:"webhook_url"`
	WebhookHeaders         types.String `tfsdk:"webhook_headers"`
	WebhookMethod          types.String `tfsdk:"webhook_method"`
	WebhookPayloadTemplate types.String `tfsdk:"webhook_payload_template"`
	WebhookTemplateKey     types.String `tfsdk:"webhook_template_key"`
	SlackWebhookURL        types.String `tfsdk:"slack_webhook_url"`
	DiscordWebhookURL      types.String `tfsdk:"discord_webhook_url"`
	TeamsWebhookURL        types.String `tfsdk:"teams_webhook_url"`
	ZoomWebhookURL         types.String `tfsdk:"zoom_webhook_url"`
	PagerDutyRoutingKey    types.String `tfsdk:"pagerduty_routing_key"`
	PagerDutySeverityMap   types.String `tfsdk:"pagerduty_severity_map"`
	PagerDutyAutoResolve   types.Bool   `tfsdk:"pagerduty_auto_resolve"`
	OpsGenieAPIKey         types.String `tfsdk:"opsgenie_api_key"`
	OpsGenieAPIURL         types.String `tfsdk:"opsgenie_api_url"`
	OpsGenieResponders     types.String `tfsdk:"opsgenie_responders"`
	OpsGeniePriorityMap    types.String `tfsdk:"opsgenie_priority_map"`
	OpsGenieAutoResolve    types.Bool   `tfsdk:"opsgenie_auto_resolve"`
	TicketProvider         types.String `tfsdk:"ticket_provider"`
	TicketBaseURL          types.String `tfsdk:"ticket_base_url"`
	TicketUserEmail        types.String `tfsdk:"ticket_user_email"`
	TicketAPIToken         types.String `tfsdk:"ticket_api_token"`
	TicketProjectKey       types.String `tfsdk:"ticket_project_key"`
	TicketIssueType        types.String `tfsdk:"ticket_issue_type"`
	TicketCloseTransition  types.String `tfsdk:"ticket_close_transition"`
	TicketAutoClose        types.Bool   `tfsdk:"ticket_auto_close"`
	ChatWebhookURL         types.String `tfsdk:"chat_webhook_url"`
	BotToken               types.String `tfsdk:"bot_token"`
	BotRecipient           types.String `tfsdk:"bot_recipient"`
	PushoverPriority       types.Int64  `tfsdk:"pushover_priority"`
	SlackSigningSecret     types.String `tfsdk:"slack_signing_secret"`
}

// --- Wire DTOs (camelCase JSON matching the alerting API) ---

type alertProfileChannelBody struct {
	Type                   string  `json:"type"`
	IsEnabled              *bool   `json:"isEnabled,omitempty"`
	Name                   *string `json:"name,omitempty"`
	EmailAddresses         *string `json:"emailAddresses,omitempty"`
	WebhookURL             *string `json:"webhookUrl,omitempty"`
	WebhookHeaders         *string `json:"webhookHeaders,omitempty"`
	WebhookMethod          *string `json:"webhookMethod,omitempty"`
	WebhookPayloadTemplate *string `json:"webhookPayloadTemplate,omitempty"`
	WebhookTemplateKey     *string `json:"webhookTemplateKey,omitempty"`
	SlackWebhookURL        *string `json:"slackWebhookUrl,omitempty"`
	DiscordWebhookURL      *string `json:"discordWebhookUrl,omitempty"`
	TeamsWebhookURL        *string `json:"teamsWebhookUrl,omitempty"`
	ZoomWebhookURL         *string `json:"zoomWebhookUrl,omitempty"`
	PagerDutyRoutingKey    *string `json:"pagerDutyRoutingKey,omitempty"`
	PagerDutySeverityMap   *string `json:"pagerDutySeverityMap,omitempty"`
	PagerDutyAutoResolve   *bool   `json:"pagerDutyAutoResolve,omitempty"`
	OpsGenieAPIKey         *string `json:"opsGenieApiKey,omitempty"`
	OpsGenieAPIURL         *string `json:"opsGenieApiUrl,omitempty"`
	OpsGenieResponders     *string `json:"opsGenieResponders,omitempty"`
	OpsGeniePriorityMap    *string `json:"opsGeniePriorityMap,omitempty"`
	OpsGenieAutoResolve    *bool   `json:"opsGenieAutoResolve,omitempty"`
	TicketProvider         *string `json:"ticketProvider,omitempty"`
	TicketBaseURL          *string `json:"ticketBaseUrl,omitempty"`
	TicketUserEmail        *string `json:"ticketUserEmail,omitempty"`
	TicketAPIToken         *string `json:"ticketApiToken,omitempty"`
	TicketProjectKey       *string `json:"ticketProjectKey,omitempty"`
	TicketIssueType        *string `json:"ticketIssueType,omitempty"`
	TicketCloseTransition  *string `json:"ticketCloseTransition,omitempty"`
	TicketAutoClose        *bool   `json:"ticketAutoClose,omitempty"`
	ChatWebhookURL         *string `json:"chatWebhookUrl,omitempty"`
	BotRecipient           *string `json:"botRecipient,omitempty"`
	PushoverPriority       *int64  `json:"pushoverPriority,omitempty"`
	// Write-only on the API: null keeps a stored value only when the channel id
	// is echoed back, which this provider never does (channels are replaced
	// wholesale), so the configured value is sent on every save.
	BotToken           *string `json:"botToken,omitempty"`
	SlackSigningSecret *string `json:"slackSigningSecret,omitempty"`
}

type alertProfileBody struct {
	Name                     *string                   `json:"name,omitempty"`
	Description              *string                   `json:"description,omitempty"`
	IsDefault                bool                      `json:"isDefault"`
	IsEnabled                *bool                     `json:"isEnabled,omitempty"`
	CooldownMinutes          *int64                    `json:"cooldownMinutes,omitempty"`
	ConsecutiveFailures      *int64                    `json:"consecutiveFailures,omitempty"`
	AlertOnRecovery          *bool                     `json:"alertOnRecovery,omitempty"`
	DeduplicateAcrossRegions *bool                     `json:"deduplicateAcrossRegions,omitempty"`
	EscalationPolicyID       *string                   `json:"escalationPolicyId,omitempty"`
	OwnerGroupID             *string                   `json:"ownerGroupId,omitempty"`
	Channels                 []alertProfileChannelBody `json:"channels,omitempty"`
}

type alertProfileChannelResponse struct {
	ID                     string  `json:"id"`
	Type                   string  `json:"type"`
	IsEnabled              bool    `json:"isEnabled"`
	Name                   *string `json:"name"`
	EmailAddresses         *string `json:"emailAddresses"`
	WebhookURL             *string `json:"webhookUrl"`
	WebhookHeaders         *string `json:"webhookHeaders"`
	WebhookMethod          *string `json:"webhookMethod"`
	WebhookPayloadTemplate *string `json:"webhookPayloadTemplate"`
	WebhookTemplateKey     *string `json:"webhookTemplateKey"`
	SlackWebhookURL        *string `json:"slackWebhookUrl"`
	DiscordWebhookURL      *string `json:"discordWebhookUrl"`
	TeamsWebhookURL        *string `json:"teamsWebhookUrl"`
	ZoomWebhookURL         *string `json:"zoomWebhookUrl"`
	PagerDutyRoutingKey    *string `json:"pagerDutyRoutingKey"`
	PagerDutySeverityMap   *string `json:"pagerDutySeverityMap"`
	PagerDutyAutoResolve   bool    `json:"pagerDutyAutoResolve"`
	OpsGenieAPIKey         *string `json:"opsGenieApiKey"`
	OpsGenieAPIURL         *string `json:"opsGenieApiUrl"`
	OpsGenieResponders     *string `json:"opsGenieResponders"`
	OpsGeniePriorityMap    *string `json:"opsGeniePriorityMap"`
	OpsGenieAutoResolve    bool    `json:"opsGenieAutoResolve"`
	TicketProvider         *string `json:"ticketProvider"`
	TicketBaseURL          *string `json:"ticketBaseUrl"`
	TicketUserEmail        *string `json:"ticketUserEmail"`
	TicketAPIToken         *string `json:"ticketApiToken"`
	TicketProjectKey       *string `json:"ticketProjectKey"`
	TicketIssueType        *string `json:"ticketIssueType"`
	TicketCloseTransition  *string `json:"ticketCloseTransition"`
	TicketAutoClose        bool    `json:"ticketAutoClose"`
	ChatWebhookURL         *string `json:"chatWebhookUrl"`
	BotRecipient           *string `json:"botRecipient"`
	PushoverPriority       *int64  `json:"pushoverPriority"`
}

type alertProfileResponse struct {
	ID                       string                        `json:"id"`
	Name                     string                        `json:"name"`
	Description              *string                       `json:"description"`
	IsDefault                bool                          `json:"isDefault"`
	IsGlobal                 bool                          `json:"isGlobal"`
	IsEnabled                bool                          `json:"isEnabled"`
	CooldownMinutes          int64                         `json:"cooldownMinutes"`
	ConsecutiveFailures      int64                         `json:"consecutiveFailures"`
	AlertOnRecovery          bool                          `json:"alertOnRecovery"`
	DeduplicateAcrossRegions bool                          `json:"deduplicateAcrossRegions"`
	EscalationPolicyID       *string                       `json:"escalationPolicyId"`
	OwnerGroupID             *string                       `json:"ownerGroupId"`
	CreatedAt                string                        `json:"createdAt"`
	UpdatedAt                string                        `json:"updatedAt"`
	Channels                 []alertProfileChannelResponse `json:"channels"`
}

func (r *alertProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_profile"
}

func (r *alertProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req, &resp.Diagnostics)
}

func (r *alertProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An alert profile controlling notification routing, cooldown, dedup behaviour " +
			"and a set of notification channels.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Alert profile ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable profile name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description.",
				Optional:            true,
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the tenant's default alert profile. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"is_global": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a global, system-managed profile (read-only).",
				Computed:            true,
			},
			"is_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the profile is enabled. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"cooldown_minutes": schema.Int64Attribute{
				MarkdownDescription: "Minutes to suppress repeat alerts after firing. Defaults to `5`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(5),
			},
			"consecutive_failures": schema.Int64Attribute{
				MarkdownDescription: "Consecutive failing checks required before alerting. Defaults to `1`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
			},
			"alert_on_recovery": schema.BoolAttribute{
				MarkdownDescription: "Send a notification when a monitor recovers. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"deduplicate_across_regions": schema.BoolAttribute{
				MarkdownDescription: "Collapse a multi-region failure into a single alert. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"escalation_policy_id": schema.StringAttribute{
				MarkdownDescription: "Optional escalation policy walked when alerts stay unacknowledged.",
				Optional:            true,
			},
			"owner_group_id": schema.StringAttribute{
				MarkdownDescription: "User group (`watchpoint_user_group`) that owns this profile. A filter label " +
					"only; it grants no access. The API rejects a group that does not exist in the tenant.",
				Optional: true,
			},
			"channels": schema.ListNestedAttribute{
				MarkdownDescription: "Notification channels attached to this profile.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							MarkdownDescription: "Channel type. One of `Email`, `Webhook`, `Slack`, `PagerDuty`, " +
								"`Discord`, `Teams`, `OpsGenie`, `Zoom`, `Ticketing`, `Telegram`, `GoogleChat`, " +
								"`Mattermost`, `Pushover`, `Webex`.",
							Required:   true,
							Validators: []validator.String{stringvalidator.OneOf(alertChannelTypes...)},
						},
						"is_enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether the channel is enabled. Defaults to `true`.",
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(true),
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Optional channel display name.",
							Optional:            true,
						},
						"email_addresses": schema.StringAttribute{
							MarkdownDescription: "Comma-separated recipient email addresses (Email channels).",
							Optional:            true,
						},
						"webhook_url": schema.StringAttribute{
							MarkdownDescription: "Generic webhook URL (Webhook channels).",
							Optional:            true,
						},
						"webhook_headers": schema.StringAttribute{
							MarkdownDescription: "JSON-encoded headers sent with the webhook.",
							Optional:            true,
						},
						"webhook_method": schema.StringAttribute{
							MarkdownDescription: "HTTP method used for the webhook.",
							Optional:            true,
						},
						"webhook_payload_template": schema.StringAttribute{
							MarkdownDescription: "Custom payload template rendered for the webhook.",
							Optional:            true,
						},
						"webhook_template_key": schema.StringAttribute{
							MarkdownDescription: "Key of a built-in payload template to use.",
							Optional:            true,
						},
						"slack_webhook_url": schema.StringAttribute{
							MarkdownDescription: "Slack incoming-webhook URL (Slack channels).",
							Optional:            true,
						},
						"discord_webhook_url": schema.StringAttribute{
							MarkdownDescription: "Discord webhook URL (Discord channels).",
							Optional:            true,
						},
						"teams_webhook_url": schema.StringAttribute{
							MarkdownDescription: "Microsoft Teams webhook URL (Teams channels).",
							Optional:            true,
						},
						"zoom_webhook_url": schema.StringAttribute{
							MarkdownDescription: "Zoom webhook URL (Zoom channels).",
							Optional:            true,
						},
						"pagerduty_routing_key": schema.StringAttribute{
							MarkdownDescription: "PagerDuty Events API routing key (PagerDuty channels).",
							Optional:            true,
							Sensitive:           true,
						},
						"pagerduty_severity_map": schema.StringAttribute{
							MarkdownDescription: "JSON map of Watchpoint severities to PagerDuty severities.",
							Optional:            true,
						},
						"pagerduty_auto_resolve": schema.BoolAttribute{
							MarkdownDescription: "Auto-resolve the PagerDuty incident on recovery. Defaults to `true`.",
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(true),
						},
						"opsgenie_api_key": schema.StringAttribute{
							MarkdownDescription: "OpsGenie API key (OpsGenie channels).",
							Optional:            true,
							Sensitive:           true,
						},
						"opsgenie_api_url": schema.StringAttribute{
							MarkdownDescription: "OpsGenie API base URL (region-specific).",
							Optional:            true,
						},
						"opsgenie_responders": schema.StringAttribute{
							MarkdownDescription: "JSON-encoded OpsGenie responders.",
							Optional:            true,
						},
						"opsgenie_priority_map": schema.StringAttribute{
							MarkdownDescription: "JSON map of Watchpoint severities to OpsGenie priorities.",
							Optional:            true,
						},
						"opsgenie_auto_resolve": schema.BoolAttribute{
							MarkdownDescription: "Auto-close the OpsGenie alert on recovery. Defaults to `true`.",
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(true),
						},
						"ticket_provider": schema.StringAttribute{
							MarkdownDescription: "Issue tracker for `Ticketing` channels: `jira`, `servicenow`, " +
								"`linear`, `github` or `zendesk`. Requires the `incident_management` feature.",
							Optional:   true,
							Validators: []validator.String{stringvalidator.OneOf("jira", "servicenow", "linear", "github", "zendesk")},
						},
						"ticket_base_url": schema.StringAttribute{
							MarkdownDescription: "Tracker base URL. For `zendesk` a bare subdomain is accepted; " +
								"`github` ignores it.",
							Optional: true,
						},
						"ticket_user_email": schema.StringAttribute{
							MarkdownDescription: "Account email used with the token (Jira, ServiceNow, Zendesk).",
							Optional:            true,
						},
						"ticket_api_token": schema.StringAttribute{
							MarkdownDescription: "Tracker API token. A `{{vault.KEY}}` reference is accepted.",
							Optional:            true,
							Sensitive:           true,
						},
						"ticket_project_key": schema.StringAttribute{
							MarkdownDescription: "Project or queue the issue is opened in. For `github` this is " +
								"`owner/repo`.",
							Optional: true,
						},
						"ticket_issue_type": schema.StringAttribute{
							MarkdownDescription: "Issue type. For `github` it is applied as labels.",
							Optional:            true,
						},
						"ticket_close_transition": schema.StringAttribute{
							MarkdownDescription: "Transition or status applied on recovery (for `zendesk`, defaults " +
								"to `solved`).",
							Optional: true,
						},
						"ticket_auto_close": schema.BoolAttribute{
							MarkdownDescription: "Close the issue when the alert recovers. Defaults to `true`.",
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(true),
						},
						"chat_webhook_url": schema.StringAttribute{
							MarkdownDescription: "Incoming-webhook URL for `GoogleChat`, `Mattermost` and `Webex` " +
								"channels. It carries a token, so it is sensitive.",
							Optional:  true,
							Sensitive: true,
						},
						"bot_token": schema.StringAttribute{
							MarkdownDescription: "Bot or application token for `Telegram` and `Pushover` channels. " +
								"Write-only on the API, so the configured value is kept in state and sent on every " +
								"save. A `{{vault.KEY}}` reference is accepted.",
							Optional:  true,
							Sensitive: true,
						},
						"bot_recipient": schema.StringAttribute{
							MarkdownDescription: "Telegram chat id, or Pushover user/group key.",
							Optional:            true,
						},
						"pushover_priority": schema.Int64Attribute{
							MarkdownDescription: "Pushover priority, `-2` to `2`. Recovery messages never go above " +
								"normal priority.",
							Optional:   true,
							Validators: []validator.Int64{int64validator.Between(-2, 2)},
						},
						"slack_signing_secret": schema.StringAttribute{
							MarkdownDescription: "Slack app signing secret that verifies interactive button clicks " +
								"(acknowledge/resolve from Slack). Write-only on the API, so the configured value " +
								"is kept in state and sent on every save.",
							Optional:  true,
							Sensitive: true,
						},
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

// alertChannelTypes mirrors the API's AlertChannelType enum names.
var alertChannelTypes = []string{
	"Email", "Webhook", "Slack", "PagerDuty", "Discord", "Teams", "OpsGenie", "Zoom", "Ticketing",
	"Telegram", "GoogleChat", "Mattermost", "Pushover", "Webex",
}

// toBody builds the create/PATCH body. On update owner_group_id is sent as ""
// when unset, the API's clear signal; null would leave the old owner in place.
func (m *alertProfileModel) toBody(update bool) *alertProfileBody {
	body := &alertProfileBody{
		Name:                     strPtr(m.Name),
		Description:              strPtr(m.Description),
		IsDefault:                m.IsDefault.ValueBool(),
		IsEnabled:                boolPtr(m.IsEnabled),
		CooldownMinutes:          int64Ptr(m.CooldownMinutes),
		ConsecutiveFailures:      int64Ptr(m.ConsecutiveFailures),
		AlertOnRecovery:          boolPtr(m.AlertOnRecovery),
		DeduplicateAcrossRegions: boolPtr(m.DeduplicateAcrossRegions),
		EscalationPolicyID:       strPtr(m.EscalationPolicyID),
		OwnerGroupID:             strPtr(m.OwnerGroupID),
	}
	if update {
		body.OwnerGroupID = strPtrClearing(m.OwnerGroupID)
	}
	if m.Channels != nil {
		body.Channels = make([]alertProfileChannelBody, 0, len(m.Channels))
		for _, c := range m.Channels {
			body.Channels = append(body.Channels, alertProfileChannelBody{
				Type:                   c.Type.ValueString(),
				IsEnabled:              boolPtr(c.IsEnabled),
				Name:                   strPtr(c.Name),
				EmailAddresses:         strPtr(c.EmailAddresses),
				WebhookURL:             strPtr(c.WebhookURL),
				WebhookHeaders:         strPtr(c.WebhookHeaders),
				WebhookMethod:          strPtr(c.WebhookMethod),
				WebhookPayloadTemplate: strPtr(c.WebhookPayloadTemplate),
				WebhookTemplateKey:     strPtr(c.WebhookTemplateKey),
				SlackWebhookURL:        strPtr(c.SlackWebhookURL),
				DiscordWebhookURL:      strPtr(c.DiscordWebhookURL),
				TeamsWebhookURL:        strPtr(c.TeamsWebhookURL),
				ZoomWebhookURL:         strPtr(c.ZoomWebhookURL),
				PagerDutyRoutingKey:    strPtr(c.PagerDutyRoutingKey),
				PagerDutySeverityMap:   strPtr(c.PagerDutySeverityMap),
				PagerDutyAutoResolve:   boolPtr(c.PagerDutyAutoResolve),
				OpsGenieAPIKey:         strPtr(c.OpsGenieAPIKey),
				OpsGenieAPIURL:         strPtr(c.OpsGenieAPIURL),
				OpsGenieResponders:     strPtr(c.OpsGenieResponders),
				OpsGeniePriorityMap:    strPtr(c.OpsGeniePriorityMap),
				OpsGenieAutoResolve:    boolPtr(c.OpsGenieAutoResolve),
				TicketProvider:         strPtr(c.TicketProvider),
				TicketBaseURL:          strPtr(c.TicketBaseURL),
				TicketUserEmail:        strPtr(c.TicketUserEmail),
				TicketAPIToken:         strPtr(c.TicketAPIToken),
				TicketProjectKey:       strPtr(c.TicketProjectKey),
				TicketIssueType:        strPtr(c.TicketIssueType),
				TicketCloseTransition:  strPtr(c.TicketCloseTransition),
				TicketAutoClose:        boolPtr(c.TicketAutoClose),
				ChatWebhookURL:         strPtr(c.ChatWebhookURL),
				BotToken:               strPtr(c.BotToken),
				BotRecipient:           strPtr(c.BotRecipient),
				PushoverPriority:       int64Ptr(c.PushoverPriority),
				SlackSigningSecret:     strPtr(c.SlackSigningSecret),
			})
		}
	}
	return body
}

// sameChannel pairs a configured channel with an API channel by type, name and
// destination, which together identify a channel within one profile.
func sameChannel(p alertProfileChannelModel, c alertProfileChannelResponse) bool {
	return p.Type.ValueString() == c.Type &&
		p.Name.Equal(strFromPtr(c.Name)) &&
		p.EmailAddresses.Equal(strFromPtr(c.EmailAddresses)) &&
		p.WebhookURL.Equal(strFromPtr(c.WebhookURL)) &&
		p.SlackWebhookURL.Equal(strFromPtr(c.SlackWebhookURL)) &&
		p.DiscordWebhookURL.Equal(strFromPtr(c.DiscordWebhookURL)) &&
		p.TeamsWebhookURL.Equal(strFromPtr(c.TeamsWebhookURL)) &&
		p.ZoomWebhookURL.Equal(strFromPtr(c.ZoomWebhookURL)) &&
		p.ChatWebhookURL.Equal(strFromPtr(c.ChatWebhookURL)) &&
		p.BotRecipient.Equal(strFromPtr(c.BotRecipient)) &&
		p.PagerDutyRoutingKey.Equal(strFromPtr(c.PagerDutyRoutingKey)) &&
		p.OpsGenieAPIKey.Equal(strFromPtr(c.OpsGenieAPIKey)) &&
		p.TicketProvider.Equal(strFromPtr(c.TicketProvider)) &&
		p.TicketProjectKey.Equal(strFromPtr(c.TicketProjectKey))
}

// applyResponse copies API response fields into the model. The OpsGenie API key
// and PagerDuty routing key are echoed back by the API. bot_token and
// slack_signing_secret are write-only, so they are carried over from the prior
// model at the same list position once the API's channels are aligned to it.
func (m *alertProfileModel) applyResponse(resp *alertProfileResponse) {
	prior := m.Channels
	m.ID = types.StringValue(resp.ID)
	m.Name = types.StringValue(resp.Name)
	m.Description = strFromPtr(resp.Description)
	m.IsDefault = types.BoolValue(resp.IsDefault)
	m.IsGlobal = types.BoolValue(resp.IsGlobal)
	m.IsEnabled = types.BoolValue(resp.IsEnabled)
	m.CooldownMinutes = types.Int64Value(resp.CooldownMinutes)
	m.ConsecutiveFailures = types.Int64Value(resp.ConsecutiveFailures)
	m.AlertOnRecovery = types.BoolValue(resp.AlertOnRecovery)
	m.DeduplicateAcrossRegions = types.BoolValue(resp.DeduplicateAcrossRegions)
	m.EscalationPolicyID = strFromPtr(resp.EscalationPolicyID)
	m.OwnerGroupID = strFromPtr(resp.OwnerGroupID)
	m.CreatedAt = types.StringValue(resp.CreatedAt)
	m.UpdatedAt = types.StringValue(resp.UpdatedAt)

	if resp.Channels == nil {
		m.Channels = nil
		return
	}
	// The API reads channels back with no ORDER BY, so they are re-paired with
	// the prior list before anything (the write-only secrets included) is
	// carried over by position.
	aligned := alignTo(prior, resp.Channels, sameChannel, func(p alertProfileChannelModel, c alertProfileChannelResponse) bool {
		return p.Type.ValueString() == c.Type
	})
	channels := make([]alertProfileChannelModel, 0, len(aligned))
	for i, c := range aligned {
		botToken, signing := types.StringNull(), types.StringNull()
		if i < len(prior) && prior[i].Type.ValueString() == c.Type {
			botToken, signing = prior[i].BotToken, prior[i].SlackSigningSecret
		}
		channels = append(channels, alertProfileChannelModel{
			Type:                   types.StringValue(c.Type),
			IsEnabled:              types.BoolValue(c.IsEnabled),
			Name:                   strFromPtr(c.Name),
			EmailAddresses:         strFromPtr(c.EmailAddresses),
			WebhookURL:             strFromPtr(c.WebhookURL),
			WebhookHeaders:         strFromPtr(c.WebhookHeaders),
			WebhookMethod:          strFromPtr(c.WebhookMethod),
			WebhookPayloadTemplate: strFromPtr(c.WebhookPayloadTemplate),
			WebhookTemplateKey:     strFromPtr(c.WebhookTemplateKey),
			SlackWebhookURL:        strFromPtr(c.SlackWebhookURL),
			DiscordWebhookURL:      strFromPtr(c.DiscordWebhookURL),
			TeamsWebhookURL:        strFromPtr(c.TeamsWebhookURL),
			ZoomWebhookURL:         strFromPtr(c.ZoomWebhookURL),
			PagerDutyRoutingKey:    strFromPtr(c.PagerDutyRoutingKey),
			PagerDutySeverityMap:   strFromPtr(c.PagerDutySeverityMap),
			PagerDutyAutoResolve:   types.BoolValue(c.PagerDutyAutoResolve),
			OpsGenieAPIKey:         strFromPtr(c.OpsGenieAPIKey),
			OpsGenieAPIURL:         strFromPtr(c.OpsGenieAPIURL),
			OpsGenieResponders:     strFromPtr(c.OpsGenieResponders),
			OpsGeniePriorityMap:    strFromPtr(c.OpsGeniePriorityMap),
			OpsGenieAutoResolve:    types.BoolValue(c.OpsGenieAutoResolve),
			TicketProvider:         strFromPtr(c.TicketProvider),
			TicketBaseURL:          strFromPtr(c.TicketBaseURL),
			TicketUserEmail:        strFromPtr(c.TicketUserEmail),
			TicketAPIToken:         strFromPtr(c.TicketAPIToken),
			TicketProjectKey:       strFromPtr(c.TicketProjectKey),
			TicketIssueType:        strFromPtr(c.TicketIssueType),
			TicketCloseTransition:  strFromPtr(c.TicketCloseTransition),
			TicketAutoClose:        types.BoolValue(c.TicketAutoClose),
			ChatWebhookURL:         strFromPtr(c.ChatWebhookURL),
			BotToken:               botToken,
			BotRecipient:           strFromPtr(c.BotRecipient),
			PushoverPriority:       int64FromPtr(c.PushoverPriority),
			SlackSigningSecret:     signing,
		})
	}
	m.Channels = channels
}

func (r *alertProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toBody(false)

	var created idNameResponse
	path := r.client.Tenantf("/alerting", "/alert-profiles")
	if err := r.client.Post(ctx, path, body, &created); err != nil {
		resp.Diagnostics.AddError("Error creating alert profile", err.Error())
		return
	}

	// POST returns only {id,name}; re-read to populate computed fields.
	var got alertProfileResponse
	getPath := r.client.Tenantf("/alerting", "/alert-profiles/%s", created.ID)
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading alert profile after create", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *alertProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var got alertProfileResponse
	getPath := r.client.Tenantf("/alerting", "/alert-profiles/%s", state.ID.ValueString())
	if err := r.client.Get(ctx, getPath, nil, &got); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading alert profile", err.Error())
		return
	}

	state.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *alertProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := plan.toBody(true)

	// PATCH returns a status message, so re-GET to refresh state.
	path := r.client.Tenantf("/alerting", "/alert-profiles/%s", plan.ID.ValueString())
	if err := r.client.Patch(ctx, path, body, nil); err != nil {
		resp.Diagnostics.AddError("Error updating alert profile", err.Error())
		return
	}

	var got alertProfileResponse
	if err := r.client.Get(ctx, path, nil, &got); err != nil {
		resp.Diagnostics.AddError("Error reading alert profile after update", err.Error())
		return
	}
	plan.applyResponse(&got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *alertProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := r.client.Tenantf("/alerting", "/alert-profiles/%s", state.ID.ValueString())
	if err := r.client.Delete(ctx, path); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting alert profile", err.Error())
	}
}

func (r *alertProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp)
}
