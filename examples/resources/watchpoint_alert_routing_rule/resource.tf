# Anything tagged database also reaches the data team. Other rules keep being
# evaluated, so an alert can land with several audiences at once.
resource "watchpoint_alert_routing_rule" "database_to_data_team" {
  name     = "Database alerts to the data team"
  priority = 10

  conditions = [
    {
      condition_type = "MonitorTag"
      operator       = "In"
      values         = ["database", "warehouse"]
    },
  ]

  targets = [
    {
      target_type = "AlertProfile"
      target_id   = watchpoint_alert_profile.data_team.id
    },
  ]
}

# Critical alerts outside office hours page the on-call lead through a
# different escalation policy. Both conditions must match (AND), and
# stop_processing keeps lower-priority rules from adding more audiences.
resource "watchpoint_alert_routing_rule" "after_hours_critical" {
  name            = "After-hours criticals to the on-call lead"
  description     = "Weeknights 22:00-06:00, New York time"
  priority        = 20
  stop_processing = true

  conditions = [
    {
      condition_type = "Severity"
      operator       = "In"
      values         = ["Critical", "Error"]
    },
    {
      condition_type      = "TimeWindow"
      operator            = "In"
      time_zone           = "America/New_York"
      days_of_week        = [1, 2, 3, 4, 5]
      start_minute_of_day = 1320 # 22:00, wrapping past midnight
      end_minute_of_day   = 360  # 06:00 the next morning
    },
  ]

  targets = [
    {
      target_type = "EscalationPolicy"
      target_id   = watchpoint_escalation_policy.platform.id
    },
    {
      target_type = "Channel"
      target_id   = "chan_pagerduty_001"
    },
  ]
}
