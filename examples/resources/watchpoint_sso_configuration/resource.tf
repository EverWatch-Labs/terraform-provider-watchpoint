resource "watchpoint_sso_configuration" "example" {
  connection_name         = "Entra ID SSO"
  provider_type           = "EntraId"
  domain                  = "example.com"
  client_id               = "your-app-id"
  oidc_authority          = "https://login.microsoftonline.com/tenant-id/v2.0"
  oidc_client_secret      = var.oidc_client_secret
  oidc_scopes             = "openid profile email"
  group_claim_name        = "groups"
  jit_provisioning_enabled = true
}
