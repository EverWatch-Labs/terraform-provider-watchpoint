# Warn 45 days before the registration expires, go critical at 10, and flag any
# registrar or nameserver change.
resource "watchpoint_domain_monitor" "primary" {
  name                 = "example.com registration"
  domain               = "example.com"
  expiry_warning_days  = 45
  expiry_critical_days = 10
}
