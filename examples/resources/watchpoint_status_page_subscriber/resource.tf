# Post incident and maintenance updates for two components into a Slack channel.
resource "watchpoint_status_page_subscriber" "eng_slack" {
  status_page_id = watchpoint_status_page.public.id
  channel        = "Slack"
  url            = var.status_slack_webhook_url
  component_ids  = [watchpoint_status_page_component.api.id, watchpoint_status_page_component.web.id]
}

# A signed webhook; verify deliveries with the secret returned on create.
resource "watchpoint_status_page_subscriber" "partner_hook" {
  status_page_id = watchpoint_status_page.public.id
  channel        = "Webhook"
  url            = "https://partner.example.com/status-events"
}
