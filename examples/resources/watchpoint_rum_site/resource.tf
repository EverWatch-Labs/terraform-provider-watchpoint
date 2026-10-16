resource "watchpoint_rum_site" "example" {
  name           = "Marketing site"
  primary_origin = "https://example.com"
  sample_rate    = 1.0
}

# Heatmaps behind a login. The screenshot capture renders the page itself, so
# without credentials a gated page is captured as its login screen. Only the
# vault KEY is stored on the site; the password stays in the vault.
resource "watchpoint_vault_secret" "app_screenshot_password" {
  key   = "RUM_SCREENSHOT_PASSWORD"
  value = var.rum_screenshot_password
}

resource "watchpoint_rum_site" "app" {
  name           = "Customer app"
  primary_origin = "https://app.example.com"

  heatmaps_enabled                   = true
  screenshot_auth_username           = "rum-capture"
  screenshot_auth_password_vault_key = watchpoint_vault_secret.app_screenshot_password.key
}
