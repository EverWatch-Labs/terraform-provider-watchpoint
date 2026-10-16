resource "watchpoint_escalation_policy" "platform" {
  name             = "Platform Escalation"
  repeat_last_step = true
  max_repeats      = 3

  steps = [
    {
      target_type        = "OnCallSchedule"
      oncall_schedule_id = watchpoint_oncall_schedule.primary.id
      delay_minutes      = 0
    },
    {
      target_type       = "User"
      target_user_email = "lead@example.com"
      delay_minutes     = 10
    },
    {
      target_type             = "AlertProfile"
      notify_alert_profile_id = watchpoint_alert_profile.devops.id
      delay_minutes           = 20
    },
  ]
}
