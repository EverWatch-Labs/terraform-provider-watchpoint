resource "watchpoint_sgl_alert_rule" "example" {
  name             = "Error spike"
  sgl_query        = "logs | where severity_number >= 17 | summarize error_count=count()"
  operator         = "gt"
  threshold        = 100
  severity         = "Critical"
  interval_seconds = 300
  alert_profile_id = "ap_001"
}
