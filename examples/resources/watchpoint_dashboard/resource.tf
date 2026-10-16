resource "watchpoint_dashboard" "example" {
  name        = "Production Overview"
  description = "Top-level production dashboard"
  layout_json = jsonencode([
    { type = "uptime", config = { monitorId = "mon_abc123" }, grid = { x = 0, y = 0, w = 4, h = 2 } },
  ])
}
