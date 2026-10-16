resource "watchpoint_monitor_group" "example" {
  name        = "Production APIs"
  description = "All production-facing API monitors"
  color       = "#3B82F6"
  icon        = "globe"
  tags        = ["team:payments", "env:prod"]
}
