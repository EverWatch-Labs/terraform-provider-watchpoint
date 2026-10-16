resource "watchpoint_status_page_component_group" "apis" {
  status_page_id       = watchpoint_status_page.public.id
  name                 = "APIs"
  description          = "Public REST and GraphQL endpoints"
  collapsed_by_default = true
}
