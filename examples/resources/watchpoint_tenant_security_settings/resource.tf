# Require MFA for every member and limit sign-in and API-key use to the office
# and VPN ranges. Declare this resource once per tenant.
resource "watchpoint_tenant_security_settings" "this" {
  enforce_mfa             = true
  ip_allowlist_enabled    = true
  allowed_ip_ranges       = ["203.0.113.0/24", "2001:db8::/48"]
  session_timeout_minutes = 120
}
