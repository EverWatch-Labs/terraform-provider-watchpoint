resource "watchpoint_sla_profile" "example" {
  name                  = "Production tier 1"
  target_uptime_percent = 99.9
  window_days           = 30
  target_p95_latency_ms = 500
  alert_on_breach       = true
  alert_profile_id      = "ap_001"
}

# A telemetry SLO: the share of checkout requests that succeeded, read from two
# OTLP metrics. Gets the same multi-window burn-rate alerts as monitor SLOs.
resource "watchpoint_sla_profile" "checkout_success" {
  name                  = "Checkout success rate"
  target_uptime_percent = 99.5
  window_days           = 28
  source_kind           = "Metrics"
  source_config_json = jsonencode({
    goodMetric  = "checkout.requests.ok"
    totalMetric = "checkout.requests"
  })
}
