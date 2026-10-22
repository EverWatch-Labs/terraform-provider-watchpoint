resource "watchpoint_maintenance_calendar_feed" "change_calendar" {
  name                  = "Change calendar"
  url                   = var.change_calendar_ics_url
  scope_type            = "group"
  scope_value           = watchpoint_monitor_group.payments.id
  summary_contains      = "[maintenance]"
  poll_interval_minutes = 30
}
