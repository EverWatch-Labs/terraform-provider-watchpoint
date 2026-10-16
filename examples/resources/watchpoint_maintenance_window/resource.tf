resource "watchpoint_maintenance_window" "example" {
  name             = "Weekly patching"
  start_time_utc   = "2026-07-05T06:00:00Z"
  end_time_utc     = "2026-07-05T08:00:00Z"
  duration_minutes = 120
  timezone         = "America/New_York"
  is_recurring     = true
  recurrence_type  = "weekly"
  recurrence_days  = "0"
  monitor_ids      = ["mon_abc123"]
}
