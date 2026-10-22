resource "watchpoint_status_page_translation" "fr" {
  status_page_id = watchpoint_status_page.public.id
  language       = "fr"

  entries = [
    { entity_type = "page", entity_id = watchpoint_status_page.public.id, field = "name", value = "Statut Acme" },
    { entity_type = "component", entity_id = watchpoint_status_page_component.api.id, field = "name", value = "API publique" },
  ]
}
