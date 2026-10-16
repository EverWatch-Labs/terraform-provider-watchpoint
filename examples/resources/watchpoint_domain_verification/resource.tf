resource "watchpoint_domain_verification" "example" {
  domain = "example.com"
  # Set verify = true to check DNS immediately after the record is published.
  verify = false
}
