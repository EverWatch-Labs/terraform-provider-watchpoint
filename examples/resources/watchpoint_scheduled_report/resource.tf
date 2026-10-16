resource "watchpoint_scheduled_report" "example" {
  name         = "Weekly Summary"
  dashboard_id = "dash_001"
  schedule     = "Weekly"
  recipients   = ["team@example.com"]
  time_range   = "7d"
}
