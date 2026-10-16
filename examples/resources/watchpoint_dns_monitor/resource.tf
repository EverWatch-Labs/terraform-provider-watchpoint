resource "watchpoint_dns_monitor" "example" {
  name            = "example.com A record"
  hostname        = "example.com"
  record_type     = "A"
  expected_values = "203.0.113.10"
  alert_on_change = true
}
