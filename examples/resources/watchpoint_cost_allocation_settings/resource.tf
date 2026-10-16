# Whether the showback report spreads the shared part of the bill across the
# tag values, or reports it in its own row. Per tenant, so declare it once
# alongside the watchpoint_cost_allocation_tag mappings it governs.
resource "watchpoint_cost_allocation_settings" "example" {
  allocate_shared_cost = true
}
