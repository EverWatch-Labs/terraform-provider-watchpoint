# OAuth 2.0 client-credentials (machine-to-machine)
resource "watchpoint_auth_profile" "payments_api" {
  name        = "PaymentsApi"
  kind        = "OAuth2ClientCredentials"
  description = "Token for the payments upstream"

  token_endpoint     = "https://auth.example.com/oauth/token"
  client_id          = "svc-monitors"
  client_secret      = var.payments_client_secret
  scopes             = "payments:read payments:write"
  audience           = "https://api.example.com"
  client_auth_method = "body"
}

# OAuth 2.0 resource-owner password grant
resource "watchpoint_auth_profile" "legacy_portal" {
  name = "LegacyPortal"
  kind = "OAuth2Password"

  token_endpoint = "https://legacy.example.com/oauth/token"
  client_id      = "portal"
  username       = "monitor@example.com"
  password       = var.legacy_password
  scopes         = "openid"
}

# OAuth 2.0 refresh-token exchange
resource "watchpoint_auth_profile" "analytics" {
  name = "Analytics"
  kind = "OAuth2RefreshToken"

  token_endpoint = "https://accounts.example.com/oauth/token"
  client_id      = "analytics-client"
  refresh_token  = var.analytics_refresh_token
}

# Custom login flow described by a JSON document
resource "watchpoint_auth_profile" "app_login" {
  name = "AppLogin"
  kind = "LoginFlow"

  login_flow_json = jsonencode({
    loginUrl      = "https://app.example.com/api/login"
    method        = "POST"
    contentType   = "application/json"
    bodyTemplate  = "{\"email\":\"{{username}}\",\"password\":\"{{password}}\"}"
    tokenJsonPath = "$.data.accessToken"
  })
}

# Static API key
resource "watchpoint_auth_profile" "status_api" {
  name = "StatusApi"
  kind = "StaticApiKey"

  static_key = var.status_api_key
}

# Static API key sourced from a vault secret
resource "watchpoint_auth_profile" "billing_api" {
  name = "BillingApi"
  kind = "StaticApiKey"

  vault_secret_ref = "BILLING_API_KEY"
}

# Mutual TLS (client certificate)
resource "watchpoint_auth_profile" "partner_mtls" {
  name = "PartnerMtls"
  kind = "Mtls"

  client_cert_pem = file("${path.module}/certs/client.crt")
  client_key_pem  = var.partner_client_key
}
