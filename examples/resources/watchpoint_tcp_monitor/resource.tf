resource "watchpoint_tcp_monitor" "example" {
  name     = "Database port"
  hostname = "db.example.com"
  port     = 5432
  use_tls  = false
}
