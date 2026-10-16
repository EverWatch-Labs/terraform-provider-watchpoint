resource "watchpoint_role" "example" {
  name        = "Read-Only Viewer"
  description = "Can view monitors and dashboards but cannot modify anything"
  permissions = ["monitors.read", "dashboards.read", "alerts.read"]
}
