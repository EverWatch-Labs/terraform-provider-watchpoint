# Federated: nothing is stored on either side. Watchpoint presents a short-lived
# assertion and AWS decides whether to honour it, so you revoke by editing the
# trust policy rather than by rotating anything here.
data "watchpoint_external_vault_federation" "aws" {
  kind = "aws-secrets-manager"
}

resource "aws_iam_openid_connect_provider" "watchpoint" {
  url            = data.watchpoint_external_vault_federation.aws.issuer
  client_id_list = [data.watchpoint_external_vault_federation.aws.audience]
}

resource "aws_iam_role" "watchpoint_secrets" {
  name = "WatchpointSecretsReader"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = aws_iam_openid_connect_provider.watchpoint.arn }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        # Pin the subject. A wildcard here would let any Watchpoint tenant
        # assume this role.
        StringEquals = {
          "${replace(data.watchpoint_external_vault_federation.aws.issuer, "https://", "")}:sub" = data.watchpoint_external_vault_federation.aws.subject
          "${replace(data.watchpoint_external_vault_federation.aws.issuer, "https://", "")}:aud" = data.watchpoint_external_vault_federation.aws.audience
        }
      }
    }]
  })
}

resource "watchpoint_external_vault" "aws" {
  slug         = "prod-aws"
  name         = "Production AWS Secrets Manager"
  kind         = "aws-secrets-manager"
  token_source = "federated"

  config = {
    region   = "us-east-1"
    role_arn = aws_iam_role.watchpoint_secrets.arn
  }
}

# Client credentials: the one mode that stores something. The credential lives in
# a Watchpoint vault secret and is named here, never inlined.
resource "watchpoint_vault_secret" "delinea_password" {
  key         = "DELINEA_SVC_PASSWORD"
  value       = var.delinea_password
  description = "Password for the Secret Server application user"
}

resource "watchpoint_external_vault" "delinea" {
  slug                  = "corp-delinea"
  name                  = "Corporate Secret Server"
  kind                  = "delinea-secret-server"
  token_source          = "client_credentials"
  credential_secret_key = watchpoint_vault_secret.delinea_password.key

  config = {
    base_url = "https://example.secretservercloud.com"
    username = "svc_watchpoint"
  }
}

# Not routable from the internet, so the fetch happens inside your network and
# the secret never transits Watchpoint at all.
resource "watchpoint_external_vault" "internal_vault" {
  slug         = "internal-vault"
  name         = "Internal HashiCorp Vault"
  kind         = "hashicorp-vault"
  token_source = "federated"
  resolution   = "agent"

  config = {
    address = "https://vault.internal.example.com:8200"
    mount   = "secret"
    role    = "watchpoint"
  }
}
