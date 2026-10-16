# Alert when a metered product's run rate rises well above this tenant's own
# trailing baseline. The relative counterpart to a usage budget's absolute
# threshold; the two are orthogonal and most tenants want both.
resource "watchpoint_usage_anomaly_alert" "example" {
  is_enabled  = true
  sensitivity = 3.0
}

resource "watchpoint_usage_budget" "overage_warning" {
  name            = "Warn at $50 overage"
  threshold_cents = 5000
  mode            = "forecast"
}
