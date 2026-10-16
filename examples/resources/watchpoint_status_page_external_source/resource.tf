# Mirror an upstream Atlassian Statuspage.
resource "watchpoint_status_page_external_source" "payments_provider" {
  status_page_id = watchpoint_status_page.example.id
  component_id   = watchpoint_status_page_component.payments.id
  kind           = "StatuspageSummary"
  url            = "https://status.stripe.com/api/v2/summary.json"
}

# Read any JSON endpoint and classify the value at a dotted path.
resource "watchpoint_status_page_external_source" "cdn" {
  status_page_id        = watchpoint_status_page.example.id
  component_id          = watchpoint_status_page_component.cdn.id
  kind                  = "HttpJson"
  url                   = "https://status.example-cdn.com/health.json"
  json_path             = "components.0.state"
  operational_values    = "ok,operational"
  degraded_values       = "degraded,partial"
  outage_values         = "down,outage"
  poll_interval_seconds = 600
}
