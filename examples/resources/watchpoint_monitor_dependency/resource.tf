resource "watchpoint_monitor_dependency" "example" {
  dependent_monitor_id  = "mon_checkout"
  depends_on_monitor_id = "mon_db"
  description           = "Checkout depends on the primary database"
}
