# Deliver alert and incident events to your own endpoint. Verify each
# delivery with the signing secret.
resource "watchpoint_event_subscription" "ops_bus" {
  name        = "Ops event bus"
  url         = "https://events.example.com/watchpoint"
  event_types = ["alert.triggered", "alert.resolved", "incident.opened"]
}

output "ops_bus_signing_secret" {
  value     = watchpoint_event_subscription.ops_bus.signing_secret
  sensitive = true
}
