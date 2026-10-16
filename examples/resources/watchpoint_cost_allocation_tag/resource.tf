# Allocate one monitor to a team for the showback report.
resource "watchpoint_cost_allocation_tag" "checkout_team" {
  tag_key       = "team"
  tag_value     = "payments"
  resource_type = "Monitor"
  resource_id   = watchpoint_http_monitor.checkout.id
}

# Tag a whole fleet under the same value with for_each. A resource carries at
# most one value per key, so a second key (env, cost-center) is a second block.
resource "watchpoint_cost_allocation_tag" "payments_fleet" {
  for_each = toset([
    watchpoint_http_monitor.checkout.id,
    watchpoint_http_monitor.refunds.id,
  ])

  tag_key       = "team"
  tag_value     = "payments"
  resource_type = "Monitor"
  resource_id   = each.value
}
