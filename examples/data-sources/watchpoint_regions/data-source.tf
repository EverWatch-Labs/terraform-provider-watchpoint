# All available monitoring regions. Use the IDs in a location profile.
data "watchpoint_regions" "all" {}

output "active_region_ids" {
  value = [for r in data.watchpoint_regions.all.regions : r.id if r.is_active]
}
