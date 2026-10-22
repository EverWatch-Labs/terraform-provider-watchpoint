resource "watchpoint_maintenance_change_source" "jira" {
  kind            = "jira"
  name            = "Jira changes"
  scope_type      = "tag"
  scope_value     = "payments"
  approved_states = ["Approved", "Scheduled"]
  start_field     = "customfield_10020"
  end_field       = "customfield_10021"
}

# Configure these in Jira's webhook settings.
output "jira_webhook_url" {
  value = watchpoint_maintenance_change_source.jira.webhook_url
}

output "jira_webhook_secret" {
  value     = watchpoint_maintenance_change_source.jira.secret
  sensitive = true
}
