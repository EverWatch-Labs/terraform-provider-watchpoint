resource "watchpoint_status_page" "example" {
  slug             = "api"
  tenant_slug      = "acme"
  name             = "API Status"
  description      = "Uptime for our public REST API"
  is_enabled       = true
  default_language = "en"
}

# An internal page only staff can open, gated on their email domain.
resource "watchpoint_status_page" "internal" {
  slug                  = "internal"
  tenant_slug           = "acme"
  name                  = "Internal Services"
  visibility            = "EmailDomainRestricted"
  allowed_email_domains = ["acme.com", "acme-contractors.com"]
}
