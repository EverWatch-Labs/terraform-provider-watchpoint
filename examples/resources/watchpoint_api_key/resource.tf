# A scoped, non-expiring key for a CI pipeline that only needs to read monitors
# and push deployment markers.
resource "watchpoint_api_key" "ci" {
  name   = "ci-pipeline"
  scopes = ["monitors.read", "deployments.write"]
}

# A key that expires, is rotated on a schedule, and starts paused until someone
# is ready to cut over to it.
resource "watchpoint_api_key" "integration" {
  name            = "partner-integration"
  scopes          = ["monitors.read", "alerts.read"]
  expires_in_days = 90
  enabled         = false

  # Change this value to issue a new secret in place. The key keeps its id, name
  # and scopes, and the previous secret stops working immediately.
  rotate_trigger = "2026-q1"
}

# A full-access key. Prefer an explicit scope list over the wildcard wherever the
# consumer's needs are known.
resource "watchpoint_api_key" "admin" {
  name   = "break-glass"
  scopes = ["*"]
}

# The secret exists only in state and in this output. Mark any output that
# carries it as sensitive, and treat the state file as a secret store.
output "ci_api_key" {
  value     = watchpoint_api_key.ci.api_key
  sensitive = true
}

# The prefix is safe to log, display, or use to identify a key in the portal.
output "ci_api_key_prefix" {
  value = watchpoint_api_key.ci.key_prefix
}
