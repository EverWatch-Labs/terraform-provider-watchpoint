resource "watchpoint_incident_update_template" "investigating" {
  name          = "Investigating (default)"
  status        = "Investigating"
  body_markdown = "We are investigating degraded performance on **{{component}}** since {{start_time}}. Next update within 30 minutes."
}
