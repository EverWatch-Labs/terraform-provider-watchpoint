# A non-human principal for CI, capped by a narrow role, with its own key.
resource "watchpoint_service_account" "ci" {
  name        = "ci-deployer"
  description = "Applies monitoring-as-code from the pipeline"
  role_id     = watchpoint_role.deployer.id
}

resource "watchpoint_api_key" "ci" {
  name               = "ci-deployer"
  service_account_id = watchpoint_service_account.ci.id
  scopes             = ["monitors.write", "checks.run"]
  allowed_ip_ranges  = ["198.51.100.0/24"]
  expires_in_days    = 90
}

output "ci_api_key" {
  value     = watchpoint_api_key.ci.api_key
  sensitive = true
}
