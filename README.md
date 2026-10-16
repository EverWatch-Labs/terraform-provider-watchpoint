# Terraform Provider for Watchpoint

Manage the [Watchpoint](https://api.everwatch-labs.io) observability platform as
code: monitors, alert/threshold/location/SLA profiles, dashboards, scheduled
reports, SGL alert rules, alert routing rules, on-call schedules and
escalation policies, maintenance windows, deployment markers, the secret vault,
upstream auth profiles, RUM sites, status pages, usage budgets, and the identity
stack (roles, user groups, SSO, SCIM-adjacent provisioning, domain verification,
audit log webhooks).

Built with the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework).

---

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- A Watchpoint **API key** (`Settings → API Keys` - the plaintext key, shown
  once, looks like `wp_live_...`) with scopes for the resources you manage.
- Your **tenant (organization) ID**.

## Provider configuration

```hcl
terraform {
  required_providers {
    watchpoint = {
      source = "everwatch-labs/watchpoint"
    }
  }
}

provider "watchpoint" {
  api_key   = var.watchpoint_api_key # or env WATCHPOINT_API_KEY
  tenant_id = var.watchpoint_tenant  # or env WATCHPOINT_TENANT_ID
  # base_url = "https://api.everwatch-labs.io/api/v1"  # or env WATCHPOINT_BASE_URL
}
```

| Argument    | Env var                | Required | Description                                                        |
|-------------|------------------------|----------|--------------------------------------------------------------------|
| `api_key`   | `WATCHPOINT_API_KEY`   | yes      | Tenant API key, sent as the `X-API-Key` header.                    |
| `tenant_id` | `WATCHPOINT_TENANT_ID` | yes      | Organization ID. Every resource is scoped to this tenant.          |
| `base_url`  | `WATCHPOINT_BASE_URL`  | no       | API root including `/api/v1`. Defaults to the production endpoint.  |

Prefer environment variables (or a secrets manager) for `api_key`. Never commit
plaintext keys.

## Quick start

```hcl
resource "watchpoint_alert_profile" "devops" {
  name   = "DevOps Team"
  emails = ["oncall@example.com"]
}

resource "watchpoint_sla_profile" "gold" {
  name                  = "Gold 99.9%"
  target_uptime_percent = 99.9
  window_days           = 30
}

resource "watchpoint_http_monitor" "api" {
  name                   = "Production API"
  url                    = "https://api.example.com/health"
  check_interval_seconds = 60
  expected_status_codes  = "200"
  alert_profile_id       = watchpoint_alert_profile.devops.id
  sla_profile_id         = watchpoint_sla_profile.gold.id
  tags                   = ["production", "public-api"]
}
```

### On-call and escalation

```hcl
resource "watchpoint_oncall_schedule" "primary" {
  name          = "Primary Rotation"
  rotation_type = "Weekly"

  participants {
    user_email = "alice@example.com"
  }
  participants {
    user_email = "bob@example.com"
  }
}

resource "watchpoint_escalation_policy" "production" {
  name = "Production Escalation"

  steps {
    target_type        = "OnCallSchedule"
    oncall_schedule_id = watchpoint_oncall_schedule.primary.id
  }
  steps {
    target_type             = "AlertProfile"
    delay_minutes           = 15
    notify_alert_profile_id = watchpoint_alert_profile.devops.id
  }
}
```

### Upstream auth profile

A `watchpoint_auth_profile` stores encrypted upstream credentials that monitors
reference through `{{auth.NAME.<field>}}` interpolation (for example
`{{auth.PaymentsApi.access_token}}`). Each profile has a single `kind`; only the
fields relevant to that kind are used.

```hcl
resource "watchpoint_auth_profile" "payments_api" {
  name        = "PaymentsApi"
  kind        = "OAuth2ClientCredentials"
  description = "Token for the payments upstream"

  token_endpoint     = "https://auth.example.com/oauth/token"
  client_id          = "svc-monitors"
  client_secret      = var.payments_client_secret
  scopes             = "payments:read payments:write"
  audience           = "https://api.example.com"
  client_auth_method = "body"
}
```

See [`examples/`](./examples) for a complete, multi-resource configuration,
including every `auth_profile` kind (`OAuth2ClientCredentials`, `OAuth2Password`,
`OAuth2RefreshToken`, `LoginFlow`, `StaticApiKey`, `Mtls`).

## Supported resources

### Monitoring
- `watchpoint_http_monitor` - HTTP/API checks with full timing breakdowns.
- `watchpoint_ssl_monitor` - certificate expiry / issuer / chain validation.
- `watchpoint_synthetic_monitor` - Playwright browser flows.
- `watchpoint_dns_monitor` - A/AAAA/CNAME/MX/NS/TXT/SOA resolution.
- `watchpoint_tcp_monitor` - TCP port connectivity (optional TLS).
- `watchpoint_ping_monitor` - ICMP round-trip / packet loss.
- `watchpoint_heartbeat_monitor` - cron / dead-man's-switch: the job pings a
  token URL, and the monitor fails when a ping does not arrive in time.

  The six probing monitor types accept an optional `sla_profile_id` to attach a
  `watchpoint_sla_profile` for uptime/latency SLO and error-budget tracking
  (requires the SLA monitoring feature). On a heartbeat monitor that attribute
  is read-only: the platform's attach endpoint does not cover heartbeats yet.
- `watchpoint_monitor_group` / `watchpoint_monitor_group_member` - logical grouping with inherited profiles.
- `watchpoint_monitor_dependency` - directed dependency edges for alert-storm suppression and blast-radius.
- `watchpoint_maintenance_window` - one-time / recurring alert suppression windows.
- `watchpoint_deployment` - deployment markers from CI/CD.
- `watchpoint_vault_secret` - encrypted secrets referenced as `{{vault.KEY}}`,
  either stored here or read through from a vault you already run.
- `watchpoint_external_vault` - a link to your own Azure Key Vault, HashiCorp
  Vault or AWS Secrets Manager. Holds endpoints and client ids only, never a
  secret value.
- `watchpoint_auth_profile` - reusable upstream credentials referenced as `{{auth.NAME.<field>}}`. One `kind` per profile: `OAuth2ClientCredentials`, `OAuth2Password`, `OAuth2RefreshToken`, `LoginFlow`, `StaticApiKey`, or `Mtls`.

### Alerting & SLAs
- `watchpoint_alert_profile` - notification channels and routing.
- `watchpoint_threshold_profile` - failure / latency conditions.
- `watchpoint_location_profile` - which regions run checks.
- `watchpoint_sla_profile` - uptime / p95 / error-rate targets.
- `watchpoint_composite_alert_rule` - **deprecated.** AND / OR / N-of-M across
  monitors. The platform no longer evaluates these rules, so an existing rule
  never fires. Express the combination as a `watchpoint_sgl_alert_rule` instead.
  The resource still applies against the retained API so shipped state keeps
  working; it will be removed in a future release.
- `watchpoint_sgl_alert_rule` - alert on a SGL query result threshold.
- `watchpoint_oncall_schedule` - daily / weekly on-call rotations.
- `watchpoint_escalation_policy` - ordered escalation steps for unacknowledged alerts.
- `watchpoint_alert_routing_rule` - route matching alerts to extra audiences
  ("database alerts also go to the data team"), with an explicit priority.
- `watchpoint_incident_severity_policy` - the response contract for one rung of
  the `Sev1`..`Sev4` ladder: acknowledgement target, who is paged, whether a
  postmortem is owed. The four rungs always exist, so this configures a rung
  rather than creating one.

### Dashboards & reports
- `watchpoint_dashboard` - SGL-backed dashboards (`layout_json` widgets).
- `watchpoint_scheduled_report` - scheduled email reports.

### Identity
- `watchpoint_role` - custom RBAC roles.
- `watchpoint_user_group` - groups with role inheritance.
- `watchpoint_domain_verification` - DNS TXT domain ownership (prerequisite for SSO).
- `watchpoint_sso_configuration` - SAML 2.0 / OIDC identity providers.
- `watchpoint_sso_group_mapping` - map IdP group claims to Watchpoint groups.
- `watchpoint_audit_webhook` - stream audit events to a SIEM (HMAC-signed).
- `watchpoint_invitation` - email invitations to the organization.
- `watchpoint_api_key` - tenant API keys, including rotation. The plaintext
  secret is returned only on create and rotate, so it lands in Terraform state.
- `watchpoint_resource_acl` - pin a role or a member to specific monitors,
  dashboards, status pages or RUM sites.

### Billing, status & RUM
- `watchpoint_usage_budget` - forecast/actual overage spend alerts against an
  absolute threshold.
- `watchpoint_usage_anomaly_alert` - the relative counterpart: alert when a
  metered product's run rate rises above the tenant's own trailing baseline.
  A per-tenant singleton.
- `watchpoint_cost_allocation_tag` - tag a billable resource for the showback
  report. One resource per (tag key, resource) mapping; `for_each` a fleet.
- `watchpoint_cost_allocation_settings` - the stored showback convention every
  report and CSV export is produced under. A per-tenant singleton.
- `watchpoint_status_page` - public status pages, including the viewer default language and the private-page audience gate.
- `watchpoint_status_page_component` - a component on a page, rolling up monitors.
- `watchpoint_status_page_maintenance` - planned work announced on a page, one-off or recurring (UTC only).
- `watchpoint_status_page_external_source` - drive a component from a third-party status page.
- `watchpoint_rum_site` - Real User Monitoring sites.

### Observability
- `watchpoint_derived_metric_rule` - emit an hourly series from spans
  (`source = "span"`) or from an existing metric (`source = "metric"`) into the
  tenant's metric rollups, where dashboards and alert rules read it like any
  ingested metric. Requires the `derived_metric_rules` feature flag.

## Data sources
- `watchpoint_regions` - available monitoring regions (use IDs in location profiles).
- `watchpoint_monitors` - list monitors for the tenant, with optional `type` / `status` filters.
- `watchpoint_otlp_hosts` - hosts seen in the tenant's OTLP host metrics over a
  rolling window, with the services on each. Stable identity only; the live CPU,
  memory and disk levels are deliberately not exposed, because a data source is
  re-read on every plan. Requires the `apm_hosts` feature flag.
- `watchpoint_external_vault_federation` - the identity Watchpoint presents to
  your external vault, for building the trust policy on the vendor side.
- Lookup-by-name data sources cover the rest, so a module can reference something
  a colleague created in the portal without importing it: `alert_profile`,
  `alert_routing_rule`, `audit_webhook`, `auth_profile`, `composite_alert_rule`,
  `dashboard`, `domain_verification`, `escalation_policy`, `location_profile`,
  `maintenance_window`, `monitor_group`, `oncall_schedule`, `role`, `rum_site`,
  `scheduled_report`, `sgl_alert_rule`, `sla_profile`, `sso_configuration`,
  `status_page`, `threshold_profile`, `usage_budget`, `user_group` and
  `vault_secret`.

## Importing existing infrastructure

Most resources import by their Watchpoint ID:

```sh
terraform import watchpoint_http_monitor.api mon_abc123
```

Composite resources use a slash-delimited ID:

```sh
terraform import watchpoint_monitor_group_member.web grp_001/mon_abc123
terraform import watchpoint_sso_group_mapping.eng sso_001/map_001
```

## Notes & behavior

- **Write-only secrets.** Fields the API never returns - `basic_auth_password`
  and `bearer_token` (HTTP monitor), `oidc_client_secret` / `saml_certificate`
  (SSO), `vault_secret.value`, and the `auth_profile` secret fields
  (`client_secret`, `password`, `refresh_token`, `static_key`, `client_cert_pem`,
  `client_key_pem`) - are not refreshed on read; Terraform keeps the configured
  value. The masked `auth_profile` response instead exposes `has_*` booleans
  (`has_client_secret`, `has_password`, ...) so out-of-band rotation is not
  drift-detected. The `audit_webhook.signing_secret` is returned once at creation
  and stored in state.
- **Defaults.** Optional attributes that the API defaults server-side are
  modeled as `Optional + Computed`, so leaving them unset adopts the platform
  default without producing perpetual diffs.
- **SLA attachment.** A monitor's `sla_profile_id` is not part of the monitor
  create/update body; the provider attaches it through a dedicated endpoint
  after the monitor is written, and only when the value is set (create) or
  changed (update), so tenants without the SLA monitoring feature never trip the
  feature gate. Setting it to `""`/`null` detaches the profile.
- **Plan tiers / feature flags.** Some resources require a plan tier or feature
  flag (e.g. synthetic monitors, dashboards, SLAs, maintenance windows, vault,
  status pages, RUM). The API returns `402`/`403` when the tenant is not
  entitled; the provider surfaces that as a create error.

## Local development

```sh
# Build
make build            # or: go build -o terraform-provider-watchpoint .

# Use the local build with a CLI dev override (~/.terraformrc or a tfrc file):
cat > dev.tfrc <<'EOF'
provider_installation {
  dev_overrides {
    "registry.terraform.io/everwatch-labs/watchpoint" = "/abs/path/to/repo"
  }
  direct {}
}
EOF
export TF_CLI_CONFIG_FILE=$PWD/dev.tfrc

# Inspect the schema
terraform providers schema -json | jq '.provider_schemas | keys'
```

Useful targets: `make fmt`, `make vet`, `make test`, `make docs`
(`docs` needs [`tfplugindocs`](https://github.com/hashicorp/terraform-plugin-docs)).

## License

Mozilla Public License 2.0 - see `LICENSE`.
