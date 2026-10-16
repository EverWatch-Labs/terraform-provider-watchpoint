data "watchpoint_external_vault_federation" "gcp" {
  kind = "gcp-secret-manager"
}

# Federation cannot work until the issuer is a public HTTPS address, because your
# cloud fetches the discovery document itself. Fail here rather than in Google's
# error message.
resource "google_iam_workload_identity_pool_provider" "watchpoint" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.watchpoint.workload_identity_pool_id
  workload_identity_pool_provider_id = "watchpoint"

  oidc {
    issuer_uri = data.watchpoint_external_vault_federation.gcp.issuer
  }

  attribute_mapping = { "google.subject" = "assertion.sub" }

  # Without this, any Watchpoint tenant could exchange a token against this pool.
  attribute_condition = "assertion.sub == '${data.watchpoint_external_vault_federation.gcp.subject}'"

  lifecycle {
    precondition {
      condition     = data.watchpoint_external_vault_federation.gcp.reachable
      error_message = "The Watchpoint issuer is not a public HTTPS address yet, so Google cannot fetch its discovery document."
    }
  }
}

# Google's audience is the pool provider path you just created, which is why it
# is a config field rather than a constant Watchpoint hands over. The data source
# says so through audience_from_field.
resource "watchpoint_external_vault" "gcp" {
  slug         = "prod-gcp"
  name         = "Production Google Secret Manager"
  kind         = "gcp-secret-manager"
  token_source = "federated"

  config = {
    project_id      = "my-project"
    audience        = "//iam.googleapis.com/${google_iam_workload_identity_pool_provider.watchpoint.name}"
    service_account = google_service_account.watchpoint_reader.email
  }
}
