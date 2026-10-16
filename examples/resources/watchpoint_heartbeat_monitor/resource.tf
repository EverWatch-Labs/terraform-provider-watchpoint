# A nightly batch job that must check in once a day. Nothing is probed: the job
# calls ping_url when it finishes, and the monitor goes critical 10 minutes
# after a daily ping fails to arrive.
resource "watchpoint_heartbeat_monitor" "nightly_invoicing" {
  name                    = "Nightly invoicing"
  expected_period_seconds = 86400
  grace_period_seconds    = 600
  alert_profile_id        = watchpoint_alert_profile.devops.id
  tags                    = ["cron", "billing"]
}

# A five-minute worker loop, with the token rotated on a schedule. Changing
# rotate_trigger mints a new token and invalidates the old URL immediately, so
# roll it out to the job in the same apply.
resource "watchpoint_heartbeat_monitor" "queue_drainer" {
  name                    = "Queue drainer"
  expected_period_seconds = 300
  grace_period_seconds    = 120
  rotate_trigger          = "2026-q3"
}

# The ping URL embeds the token, which is the only credential on an anonymous
# endpoint. Mark every output that carries it sensitive.
output "nightly_invoicing_ping_url" {
  value     = watchpoint_heartbeat_monitor.nightly_invoicing.ping_url
  sensitive = true
}
