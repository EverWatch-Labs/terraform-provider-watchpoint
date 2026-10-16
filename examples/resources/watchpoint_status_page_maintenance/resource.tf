# One-off window.
resource "watchpoint_status_page_maintenance" "db_upgrade" {
  status_page_id       = watchpoint_status_page.example.id
  title                = "Database upgrade"
  description_markdown = "We are moving the primary database to a larger instance. Expect brief write errors."
  scheduled_start_at   = "2026-07-04T02:00:00Z"
  scheduled_end_at     = "2026-07-04T04:00:00Z"
  component_ids        = [watchpoint_status_page_component.api.id]
}

# Recurring window: every other week on Monday and Wednesday at 02:00 UTC,
# for twelve occurrences.
resource "watchpoint_status_page_maintenance" "patching" {
  status_page_id     = watchpoint_status_page.example.id
  title              = "Routine patching"
  scheduled_start_at = "2026-07-06T02:00:00Z"
  scheduled_end_at   = "2026-07-06T03:00:00Z"
  suppress_alerts    = true
  component_ids      = [watchpoint_status_page_component.api.id]

  recurrence_frequency = "Weekly"
  recurrence_interval  = 2
  recurrence_by_day    = ["MO", "WE"]
  recurrence_count     = 12
}
