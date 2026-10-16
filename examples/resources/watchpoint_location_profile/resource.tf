data "watchpoint_regions" "all" {}

resource "watchpoint_location_profile" "example" {
  name       = "US + EU"
  region_ids = [for r in data.watchpoint_regions.all.regions : r.id if r.is_active]
  is_default = true
}
