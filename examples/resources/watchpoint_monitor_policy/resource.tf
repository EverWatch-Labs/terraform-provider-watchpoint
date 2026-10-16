# Only page when at least two regions agree the API is down, and file the
# monitor under the platform team.
resource "watchpoint_monitor_policy" "api" {
  monitor_id          = watchpoint_http_monitor.api.id
  confirmation_policy = "Quorum"
  quorum_min_regions  = 2
  owner_group_id      = watchpoint_user_group.platform.id
}
