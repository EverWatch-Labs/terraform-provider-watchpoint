# A representative end-to-end Watchpoint configuration. It wires regions ->
# location profile -> profiles -> monitors -> group -> SLA / composite alerts,
# plus a maintenance window, vault secret, dashboard, status page and a CI
# deployment marker. Trim to taste.

terraform {
  required_providers {
    watchpoint = {
      source = "everwatch-labs/watchpoint"
    }
  }
}

provider "watchpoint" {
  # api_key / tenant_id read from WATCHPOINT_API_KEY / WATCHPOINT_TENANT_ID
}

# ─── Discover regions ───────────────────────────────────────────────────────
data "watchpoint_regions" "all" {}

locals {
  # First two active regions, by slug, for the location profile.
  selected_region_ids = [
    for r in data.watchpoint_regions.all.regions : r.id
    if r.is_active
  ]
}

# ─── Profiles ───────────────────────────────────────────────────────────────
resource "watchpoint_location_profile" "primary" {
  name       = "US + EU"
  region_ids = slice(local.selected_region_ids, 0, min(2, length(local.selected_region_ids)))
  is_default = true
}

resource "watchpoint_threshold_profile" "strict" {
  name = "Strict"
  rules = [
    {
      metric               = "ResponseTimeMs"
      operator             = "GreaterThan"
      warning_value        = 2000
      critical_value       = 5000
      consecutive_breaches = 2
    },
  ]
}

resource "watchpoint_alert_profile" "devops" {
  name              = "DevOps Team"
  cooldown_minutes  = 15
  alert_on_recovery = true

  channels = [
    {
      type            = "Email"
      email_addresses = "oncall@example.com,team@example.com"
    },
    {
      type              = "Slack"
      slack_webhook_url = "https://hooks.slack.com/services/T00/B00/XXX"
    },
  ]
}

resource "watchpoint_sla_profile" "tier1" {
  name                 = "Production tier 1"
  target_uptime_percent = 99.9
  window_days          = 30
  target_p95_latency_ms = 500
  alert_on_breach      = true
  alert_profile_id     = watchpoint_alert_profile.devops.id
}

# ─── Secret vault ───────────────────────────────────────────────────────────
resource "watchpoint_vault_secret" "api_token" {
  key         = "HEALTHCHECK_TOKEN"
  value       = var.healthcheck_token
  description = "Bearer token used by the health-check monitor"
  category    = "api-keys"
}

# ─── Monitors ───────────────────────────────────────────────────────────────
resource "watchpoint_http_monitor" "api" {
  name                   = "Production API"
  url                    = "https://api.example.com/health"
  method                 = "GET"
  check_interval_seconds = 60
  timeout_seconds        = 30
  expected_status_codes  = "200"
  expected_body_contains = "\"status\":\"ok\""
  bearer_token           = "{{vault.HEALTHCHECK_TOKEN}}"
  validate_ssl           = true

  location_profile_id  = watchpoint_location_profile.primary.id
  threshold_profile_id = watchpoint_threshold_profile.strict.id
  alert_profile_id     = watchpoint_alert_profile.devops.id
  tags                 = ["production", "public-api"]
}

resource "watchpoint_ssl_monitor" "cert" {
  name                = "example.com TLS"
  hostname            = "example.com"
  port                = 443
  expiry_warning_days = 30
  expiry_critical_days = 7
  alert_profile_id    = watchpoint_alert_profile.devops.id
  tags                = ["tls"]
}

resource "watchpoint_dns_monitor" "apex" {
  name             = "example.com A record"
  hostname         = "example.com"
  record_type      = "A"
  alert_profile_id = watchpoint_alert_profile.devops.id
}

# ─── Grouping & dependencies ────────────────────────────────────────────────
resource "watchpoint_monitor_group" "production" {
  name             = "Production APIs"
  description      = "All production-facing API monitors"
  color            = "#3B82F6"
  icon             = "globe"
  alert_profile_id = watchpoint_alert_profile.devops.id
  sla_profile_id   = watchpoint_sla_profile.tier1.id
}

resource "watchpoint_monitor_group_member" "api_in_prod" {
  group_id     = watchpoint_monitor_group.production.id
  monitor_id   = watchpoint_http_monitor.api.id
  monitor_type = "HttpApi"
}

# ─── Composite & SGL alerting ───────────────────────────────────────────────
resource "watchpoint_composite_alert_rule" "core_down" {
  name             = "Core services degraded"
  logic            = "Or"
  alert_profile_id = watchpoint_alert_profile.devops.id

  conditions = [
    { monitor_id = watchpoint_http_monitor.api.id, monitor_type = "HttpApi", fail_status = "Critical" },
    { monitor_id = watchpoint_ssl_monitor.cert.id, monitor_type = "Ssl", fail_status = "Warning" },
  ]
}

resource "watchpoint_sgl_alert_rule" "error_spike" {
  name             = "Error spike"
  sgl_query        = "logs | where severity_number >= 17 | summarize error_count=count()"
  operator         = "gt"
  threshold        = 100
  severity         = "Critical"
  interval_seconds = 300
  alert_profile_id = watchpoint_alert_profile.devops.id
}

# ─── Maintenance window ─────────────────────────────────────────────────────
resource "watchpoint_maintenance_window" "weekly_patch" {
  name             = "Weekly patching"
  start_time_utc   = "2026-07-05T06:00:00Z"
  end_time_utc     = "2026-07-05T08:00:00Z"
  duration_minutes = 120
  timezone         = "America/New_York"
  is_recurring     = true
  recurrence_type  = "weekly"
  recurrence_days  = "0"
  monitor_ids      = [watchpoint_http_monitor.api.id]
}

# ─── Dashboard & status page ────────────────────────────────────────────────
resource "watchpoint_dashboard" "overview" {
  name        = "Production Overview"
  description = "Top-level production dashboard"
  layout_json = jsonencode([
    { type = "uptime", config = { monitorId = watchpoint_http_monitor.api.id }, grid = { x = 0, y = 0, w = 4, h = 2 } },
  ])
}

resource "watchpoint_status_page" "public" {
  slug                = "api"
  tenant_slug         = var.tenant_slug
  name                = "API Status"
  description         = "Uptime for our public REST API"
  primary_dashboard_id = watchpoint_dashboard.overview.id
  is_enabled          = true
}

# ─── RUM & budgets ──────────────────────────────────────────────────────────
resource "watchpoint_rum_site" "marketing" {
  name           = "Marketing site"
  primary_origin = "https://example.com"
  sample_rate    = 1.0
}

resource "watchpoint_usage_budget" "overage" {
  name           = "Warn at $50 overage"
  threshold_cents = 5000
  mode           = "forecast"
}

# ─── Deployment marker (call from CI) ───────────────────────────────────────
resource "watchpoint_deployment" "release" {
  version     = var.release_version
  service     = "api-server"
  environment = "production"
  monitor_ids = [watchpoint_http_monitor.api.id]
}

output "rum_public_key" {
  value = watchpoint_rum_site.marketing.public_key
}
