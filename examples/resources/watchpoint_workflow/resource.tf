# Post to an incident webhook whenever a critical alert fires, at most once
# every 15 minutes.
resource "watchpoint_workflow" "page_on_critical" {
  name             = "Critical alerts to the incident bridge"
  trigger          = "alert.fired"
  cooldown_minutes = 15
  condition_json   = jsonencode({ severity = "Critical" })

  actions = [
    {
      action_type = "Webhook"
      config_json = jsonencode({ url = "https://hooks.example.com/incident-bridge" })
    },
  ]
}
