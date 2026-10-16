resource "watchpoint_status_page_component" "api" {
  status_page_id   = watchpoint_status_page.example.id
  name             = "Public API"
  description      = "REST and GraphQL endpoints"
  position         = 0
  aggregation_mode = "WorstOf"

  monitor_ids = [
    watchpoint_http_monitor.api_health.id,
    watchpoint_http_monitor.api_graphql.id,
  ]
}

resource "watchpoint_status_page_component" "dashboard" {
  status_page_id = watchpoint_status_page.example.id
  name           = "Dashboard"
  position       = 1

  # The dashboard is unusable when the API is down, so an API outage explains
  # this component going red too.
  depends_on_component_ids = [watchpoint_status_page_component.api.id]
}
