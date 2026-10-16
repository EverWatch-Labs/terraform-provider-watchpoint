resource "watchpoint_usage_budget" "example" {
  name            = "Warn at $50 overage"
  threshold_cents = 5000
  mode            = "forecast"
}
