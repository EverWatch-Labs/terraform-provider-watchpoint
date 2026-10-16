resource "watchpoint_ssl_monitor" "example" {
  name                 = "example.com TLS"
  hostname             = "example.com"
  port                 = 443
  expiry_warning_days  = 30
  expiry_critical_days = 7
}
