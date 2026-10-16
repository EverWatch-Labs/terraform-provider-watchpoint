resource "watchpoint_alert_profile" "example" {
  name              = "DevOps Team"
  cooldown_minutes  = 15
  alert_on_recovery = true

  channels = [
    {
      type            = "Email"
      email_addresses = "oncall@example.com,team@example.com"
    },
    {
      type              = "Slack"
      slack_webhook_url = "https://hooks.slack.com/services/T00/B00/XXX"
    },
  ]
}
