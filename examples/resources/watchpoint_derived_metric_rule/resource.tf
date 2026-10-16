# A span-derived metric: how many checkout spans, broken down by status and route.
# The emitted series is "span.checkout.errors" - reference metric_name, not name.
resource "watchpoint_derived_metric_rule" "checkout_errors" {
  name    = "span.checkout.errors"
  source  = "span"
  service = "checkout-api"

  labels = ["status", "http.route"]

  filters = {
    "http.request.method" = "POST"
  }
}

# A recording rule: fold an expensive series to one precomputed value per hour so
# a dashboard or an alert reads a single row instead of re-aggregating.
resource "watchpoint_derived_metric_rule" "checkout_p95" {
  name        = "rule.checkout.latency.p95"
  source      = "metric"
  metric      = "http.server.request.duration"
  aggregation = "p95"
}

output "checkout_error_metric" {
  value = watchpoint_derived_metric_rule.checkout_errors.metric_name
}
