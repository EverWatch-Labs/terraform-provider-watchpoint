resource "watchpoint_vault_secret" "example" {
  key         = "STRIPE_API_KEY"
  value       = var.stripe_api_key
  description = "Production Stripe secret"
  category    = "api-keys"
}

# Or link the secret instead of storing it. Watchpoint keeps the reference and
# reads the value from your own vault when a check needs it, so the value never
# lands in Terraform state either. Rotating it on your side takes effect with no
# Terraform change at all, because the version is left unpinned.
resource "watchpoint_vault_secret" "linked_stripe_key" {
  key         = "STRIPE_API_KEY_LINKED"
  description = "Read from our own AWS Secrets Manager"

  external_vault = watchpoint_external_vault.aws.slug
  external_name  = "prod/stripe"
  external_field = "api_key"
}
