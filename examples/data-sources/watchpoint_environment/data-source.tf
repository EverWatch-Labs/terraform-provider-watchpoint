data "watchpoint_environment" "default" {
  key = "default"
}

resource "watchpoint_ping_monitor" "edge" {
  name           = "Edge router"
  hostname       = "203.0.113.10"
  environment_id = data.watchpoint_environment.default.id
}
