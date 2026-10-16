# Changelog

## 1.1.0 (Unreleased)

FEATURES:

* **New Resource:** `watchpoint_agent_enrollment_token` - mints a bounded-use
  agent enrollment token (Region API, `/agents/enrollment-tokens`). `token` and
  `install_commands` are Sensitive and returned only at creation. Every argument
  forces replacement; destroy revokes the token. A spent or expired token stays
  in state so dependent resources are not replaced after an install.
* **New Resource:** `watchpoint_heartbeat_monitor` - a cron / dead-man's-switch
  monitor (Monitoring, `/monitors`). The job pings a token URL, and the monitor
  fails when a ping does not arrive within `expected_period_seconds` plus
  `grace_period_seconds`. `ping_url` also accepts `/start` and `/fail` suffixes.
  There is deliberately no interval, timeout or target attribute, because a
  heartbeat is never probed. `cron_expression` and `time_zone` put the job on a
  five-field cron schedule in an IANA zone instead of a fixed period, so a
  weekday job stays quiet at the weekend; `next_expected_at` reports the run the
  monitor is waiting on. `sla_profile_id` is settable, through the same attach
  endpoint every other monitor type uses.
* **New Resources:** `watchpoint_mail_monitor` (SMTP, IMAP or POP3 with
  STARTTLS or implicit TLS, an optional login whose password is write-only, and
  banner and response-time assertions), `watchpoint_udp_monitor` (a text or hex
  datagram with a contains or regex expectation on the reply) and
  `watchpoint_domain_monitor` (registration expiry thresholds plus registrar and
  nameserver change warnings, read over RDAP with a WHOIS fallback). All three
  run on hosted locations only. Requires the `mail_monitors`, `udp_monitors` and
  `domain_monitors` features respectively.
* **New Resource:** `watchpoint_monitor_policy` - cross-region down
  confirmation (`None`, `ConfirmFromAnotherRegion` or `Quorum` with
  `quorum_min_regions`) and the owning team for one monitor of any type,
  including a Kubernetes cluster monitor. Destroying it resets confirmation to
  `None` and clears the owner.
* **New Resource:** `watchpoint_workflow` - runs Slack, webhook, PagerDuty,
  Teams, email or ticket-reply actions when an alert or incident event fires
  (Alerting, `/workflows`), with an optional condition and cooldown.
* **New Resource:** `watchpoint_event_subscription` - delivers platform events
  (for example `alert.triggered`, `incident.opened`) to an HTTPS endpoint,
  signed with an HMAC secret that is returned on create and rotated through
  `rotate_trigger`.
* **New Resource:** `watchpoint_tenant_security_settings` - the tenant's MFA
  enforcement, sign-in and API-key IP allowlist, and idle session timeout. A
  per-tenant singleton.
* **New Resource:** `watchpoint_service_account` - a non-human principal that
  holds one role and its own API keys. It never counts as a seat and cannot sign
  in. Mint its keys with `watchpoint_api_key` and `service_account_id`; the key
  secret is sensitive and stored in state.
* **New Resources:** `watchpoint_status_page_component_group` (a collapsible,
  ordered heading on a status page; imports as `<status_page_id>/<group_id>`),
  `watchpoint_status_page_subscriber` (a signed webhook, Slack or Microsoft
  Teams subscriber added by an admin and confirmed immediately) and
  `watchpoint_incident_update_template` (reusable incident update text with
  `{{component}}`, `{{start_time}}` and similar variables).
* **New Resources:** `watchpoint_log_pipeline` (ordered ingest processors and
  tenant PII redaction rules, a per-tenant singleton; processors require the
  `log_pipelines` feature), `watchpoint_log_archive_destination` (Azure Blob or
  S3-compatible archive of closed log days as gzip JSONL; requires
  `log_archives`) and `watchpoint_cloud_integration` (AWS CloudWatch, Azure
  Monitor or GCP Cloud Monitoring metrics pulled every five minutes; requires
  `cloud_integrations`). Every credential is write-only on the API, so the
  configured value is kept in state and a change made in the portal is not
  detected.
* **New Resource:** `watchpoint_dashboard_folder` - folders for custom
  dashboards, nested one level deep. Destroying a folder keeps its contents:
  dashboards move up to the parent folder and subfolders move to the root.
* **New Data Source:** `watchpoint_annotations` - chart annotations (deploy and
  config markers, hand-placed notes) in a time window, optionally scoped to one
  monitor or service.
* **New Data Source:** `watchpoint_kubernetes_cluster` - the cluster monitor
  the platform creates when a Kubernetes operator enrolls, looked up by the
  operator's `clusterName`. The monitor is created and removed with the
  operator, so it is not a resource; use its `id` with
  `watchpoint_monitor_policy` and the other monitor references. Live node and
  pod counts are deliberately omitted so the data source does not churn plans.
* **New Resource:** `watchpoint_alert_routing_rule` - routes a matching alert to
  extra audiences (Alerting, `/alerting/routing-rules`), evaluated in `priority`
  order. Rules add reach and never take it away: an alert that matches no rule
  still goes out through the monitor's alert profile.
* **New Resource:** `watchpoint_incident_severity_policy` - the response
  contract for one rung of the `Sev1` to `Sev4` ladder (Alerting): how fast an
  incident must be picked up, who is paged, whether a postmortem is owed. The
  four rungs always exist, so this configures a rung rather than creating one,
  and destroying it leaves the stored override in force. Out-of-range values are
  rejected at plan time rather than being silently clamped by the API.
* **New Resource:** `watchpoint_external_vault` - a link to an Azure Key Vault,
  HashiCorp Vault or AWS Secrets Manager you already run (Monitoring,
  `/vault/external`), so a `watchpoint_vault_secret` can read through to it
  instead of storing a copy. Endpoints, directory and client ids only; no secret
  value is stored in the resource or in state.
* **New Resource:** `watchpoint_api_key` - tenant API keys, including rotation
  (Identity, `/api-keys`). The plaintext secret is returned only on create and on
  rotation, so it is written to Terraform state and must be treated as a secret.
  `name`, `scopes` and `expires_in_days` replace the key; `enabled` and
  `rotate_trigger` apply in place. The credential driving this resource needs
  both `apikeys.manage` and `settings.manage`, because the latter is what lists
  keys and so is how the resource reads its own state.
* **New Resource:** `watchpoint_resource_acl` - pins one role or one member to
  specific monitors, dashboards, status pages or RUM sites (Identity,
  `/access-control`). Narrowing fails open, so destroying the resource widens the
  subject's access rather than removing it. `patterns` is the complete set for a
  subject and kind: a grant added outside Terraform is removed on the next apply.
* **New Resource:** `watchpoint_usage_anomaly_alert` - the relative counterpart
  to `watchpoint_usage_budget`. Notifies the tenant's billing recipients when a
  metered product's run rate rises above that tenant's own trailing baseline
  (Billing, `/billing/usage-anomaly`), which also catches the tenant whose normal
  spend already sits above every threshold they would have set. A per-tenant
  singleton covering every metered product. `sensitivity` is validated to the
  API's own `[1.5, 6.0]` window at plan time rather than being stored as the
  nearest bound. Destroying it disables the alerts rather than leaving them
  paging.
* **New Resource:** `watchpoint_cost_allocation_tag` - manages a cost-allocation
  tag on one billable resource (Billing, `/billing/cost-tags`), the mapping the
  showback report apportions each product's spend across. One resource per
  mapping, matching the API's `(tag_key, resource_type, resource_id)` identity;
  use `for_each` to tag a fleet under one value. Only `tag_value` is mutable.
* **New Resource:** `watchpoint_cost_allocation_settings` - the stored showback
  convention every cost report and CSV export is produced under (Billing,
  `/billing/cost-tags/settings`). A per-tenant singleton, declared alongside the
  `watchpoint_cost_allocation_tag` mappings it governs. The report's
  `allocateSharedCost` query parameter is a what-if preview that never writes the
  stored value, so a colleague toggling it in the portal does not show up as
  drift.
* **New Resource:** `watchpoint_status_page_component` - one component on a
  status page (Status, `/status/pages/{id}/components`), rolling up the health of
  the monitors attached to it. `monitor_ids` and `depends_on_component_ids` are
  sets and are always sent whole, so removing one from configuration clears it
  server-side. Imports as `<status_page_id>/<component_id>`.
* **New Resource:** `watchpoint_status_page_maintenance` - planned work
  announced on a status page, one-off or recurring (Status). Every instant is
  UTC and there is no timezone field, so a series scheduled at 02:00 stays at
  02:00 across a daylight-saving change. This is the status-page announcement,
  not the monitor-pausing window: `watchpoint_maintenance_window` remains the
  resource that pauses monitors.
* **New Resource:** `watchpoint_status_page_external_source` - drives one status
  page component from a third-party status page instead of from your own monitors
  (Status), so an upstream provider you depend on appears on your page. A failing
  probe reads `Unknown`; it never turns a component red on its own. Requires the
  `status_third_party_components` feature flag.
* **New Resource:** `watchpoint_derived_metric_rule` - emits an hourly series
  from spans (`source = "span"`) or from an existing metric
  (`source = "metric"`) into the tenant's metric rollups, where dashboards, the
  metrics explorer and alert rules read it like any ingested metric
  (Observability). Only `enabled` changes in place; every other attribute
  replaces the rule. Requires the `derived_metric_rules` feature flag, and a key
  with the broad `settings.manage` scope because these routes match no fragment
  in the API key path-scope map.
* **New Data Source:** `watchpoint_otlp_hosts` - the hosts the tenant's OTLP host
  metrics have been observed from in a rolling window, with the services seen on
  each. Stable identity only: the live CPU, memory, disk and network levels the
  same endpoint carries are deliberately omitted, because a data source is
  re-read on every plan and a number that moves every minute would churn anything
  built from it. Requires the `apm_hosts` feature flag.
* **New Data Source:** `watchpoint_external_vault_federation` - the identity
  Watchpoint presents to an external vault, for building the trust policy on the
  vendor side from the same values Watchpoint will actually present.
* **New Data Sources:** lookup-by-name data sources, so a module can reference
  something a colleague created in the portal without importing it:
  `watchpoint_alert_profile`, `watchpoint_alert_routing_rule`,
  `watchpoint_audit_webhook`, `watchpoint_auth_profile`,
  `watchpoint_composite_alert_rule`, `watchpoint_dashboard`,
  `watchpoint_domain_verification`, `watchpoint_escalation_policy`,
  `watchpoint_location_profile`, `watchpoint_maintenance_window`,
  `watchpoint_monitor_group`, `watchpoint_oncall_schedule`, `watchpoint_role`,
  `watchpoint_rum_site`, `watchpoint_scheduled_report`,
  `watchpoint_sgl_alert_rule`, `watchpoint_sla_profile`,
  `watchpoint_sso_configuration`, `watchpoint_status_page`,
  `watchpoint_threshold_profile`, `watchpoint_usage_budget`,
  `watchpoint_user_group` and `watchpoint_vault_secret`.

ENHANCEMENTS:

* resource/watchpoint_alert_profile: channel `type` now accepts `Ticketing`,
  `Telegram`, `GoogleChat`, `Mattermost`, `Pushover` and `Webex`, validated at
  plan time. New channel attributes: `ticket_provider` (`jira`, `servicenow`,
  `linear`, `github`, `zendesk`) with the other `ticket_*` fields,
  `chat_webhook_url`, `bot_token`, `bot_recipient`, `pushover_priority` and
  `slack_signing_secret` (verifies acknowledge and resolve clicks from Slack).
  `bot_token` and `slack_signing_secret` are write-only on the API, so the
  configured value is kept in state and sent on every save. Adds
  `owner_group_id`.
* resource/watchpoint_oncall_schedule: adds `time_zone` (IANA; handoffs follow
  daylight-saving changes), `rotation_type = "Custom"` with
  `rotation_length_hours`, `restrictions` (weekday and minute windows,
  overnight windows wrap past midnight) and `layers` (extra rotations above the
  base, each with its own cadence, restrictions and people; the first active,
  staffed layer wins). Removing `layers`, `restrictions` or
  `rotation_length_hours` from configuration clears them on the server.
* resource/watchpoint_sla_profile: adds `source_kind` (`Monitor`, `Traces` or
  `Metrics`) and `source_config_json`, so an SLO can be scored from OTLP spans or
  from the ratio of two OTLP metrics, with the same burn-rate alerts as monitor
  SLOs. Requires the `slo_trace_source` feature.
* resource/watchpoint_alert_routing_rule, resource/watchpoint_sgl_alert_rule
  and resource/watchpoint_dashboard: add `owner_group_id`, the owning team as a
  filter label. Removing it from configuration clears it.
* resource/watchpoint_dashboard: adds `folder_id`.
* resource/watchpoint_api_key: adds `allowed_ip_ranges`, a per-key CIDR
  allowlist applied on top of the tenant allowlist, and `service_account_id`.
  `scopes` is now required, matching the API, which no longer turns a blank
  list into full access.
* resource/watchpoint_dns_monitor: adds the `CAA` record type,
  `require_dnssec` and `compare_nameservers`.
* resource/watchpoint_status_page: adds `custom_css`, scoped and sanitized by
  the API (requires the `status_custom_css` feature).
* resource/watchpoint_status_page_component: adds `group_id`.

* resource/watchpoint_monitor_group: adds `tags` (set of string), inherited by
  every monitor in the group. `key:value` entries group cost-allocation spend by
  that key, a plain entry is a label. A tag set on a monitor overrides the
  group's value for the same key. Requires a platform with group tag support.

* resource/watchpoint_status_page: adds `default_language` (`en`, `es` or `de`,
  mirroring the set the API supports), `access_password` and
  `allowed_email_domains`, and promotes `visibility` from read-only to
  configurable (`Public`, `PasswordProtected` or `EmailDomainRestricted`;
  `TokenProtected` is rejected by the API and so is not offered). A read-only
  `has_access_password` reports whether a password is set. `access_password` is
  never returned by the API, so it is never refreshed on read and an imported
  page carries no password in state; set it in configuration to bring it under
  management. Because the create endpoint accepts none of these, Create now POSTs
  and then PATCHes, which also keeps a visibility switch and its credential in
  one request, as the API's own lock-out guard requires.
* resource/watchpoint_vault_secret: adds `engine`, `external_vault`,
  `external_name`, `external_field` and `external_version`, so a secret can be
  read through from a `watchpoint_external_vault` instead of being stored in
  Watchpoint.
* resource/watchpoint_rum_site: adds `screenshot_auth_username` and
  `screenshot_auth_password_vault_key`, the credentials screenshot capture
  presents to a site behind a login.
* resource/watchpoint_monitor_group: `color` and `icon` are now validated at plan
  time, against `#RRGGBB` and against the icon set the portal can actually
  render, instead of being accepted and then falling back to the default folder
  icon at render time.

BUG FIXES:

* resource/watchpoint_oncall_schedule: a participant given only `user_email`
  read back with `user_id = ""`, so the apply failed with an inconsistent
  result. An empty id now reads back as null.

* resource/watchpoint_http_monitor, resource/watchpoint_ssl_monitor,
  resource/watchpoint_synthetic_monitor, resource/watchpoint_dns_monitor,
  resource/watchpoint_tcp_monitor and resource/watchpoint_ping_monitor: removing
  `tags` from config was a silent no-op, because an absent set was omitted from
  the PATCH body and the API reads an omitted field as "leave unchanged". The
  update path now sends the empty string, which is the API's clear signal. A
  config that previously left stale tags on the monitor will clear them on the
  next apply.

* resource/watchpoint_alert_profile, resource/watchpoint_escalation_policy,
  resource/watchpoint_oncall_schedule, resource/watchpoint_sgl_alert_rule,
  resource/watchpoint_dashboard and resource/watchpoint_scheduled_report: these
  addressed the `/monitoring` service prefix for paths the API gateway then
  re-dispatched. They now call the owning service directly, `/alerting` and
  `/analytics`. The gateway still rewrites the old paths for provider versions
  already in the wild, so upgrading is backward compatible.

NOTES:

* resource/watchpoint_alert_profile `channels`,
  resource/watchpoint_escalation_policy `steps`,
  resource/watchpoint_oncall_schedule `participants` and
  resource/watchpoint_threshold_profile `rules` no longer export a nested,
  server-assigned `id`. Each of these collections is replaced wholesale on
  update, so the nested id was never a stable handle to reference. Remove any
  reference to it before upgrading.
* resource/watchpoint_composite_alert_rule: **Deprecated.** The platform no
  longer evaluates composite alert rules, so an existing rule never fires.
  Express the same combination as an SGL alert rule and use
  `watchpoint_sgl_alert_rule` (Alerting, `/alerting/rules`) instead. The
  resource, its schema and its CRUD are unchanged and still apply against the
  retained API, so no configuration breaks on upgrade; `terraform plan` and
  `terraform apply` now emit a deprecation warning. The resource will be removed
  in a future release, after which the API endpoint is retired. The
  documentation page carries a side-by-side migration example that translates
  the conditions, the `logic` / `min_fail_count` combination and the
  `fail_status` set into the equivalent `watchpoint_sgl_alert_rule`.
* data-source/watchpoint_composite_alert_rule: **Deprecated.** Same reason and
  same replacement as the resource above.

## 1.0.0

Initial release.
