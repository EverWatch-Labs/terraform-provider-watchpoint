resource "watchpoint_ping_monitor" "example" {
  name                    = "Edge router"
  hostname                = "203.0.113.1"
  ping_count              = 4
  max_packet_loss_percent = 10
}
