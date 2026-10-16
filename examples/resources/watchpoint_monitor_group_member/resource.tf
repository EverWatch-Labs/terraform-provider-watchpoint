resource "watchpoint_monitor_group_member" "example" {
  group_id     = "grp_001"
  monitor_id   = "mon_abc123"
  monitor_type = "HttpApi"
}
