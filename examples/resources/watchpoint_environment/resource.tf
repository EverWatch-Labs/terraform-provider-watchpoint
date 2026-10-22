resource "watchpoint_environment" "production" {
  key           = "production"
  name          = "Production"
  color         = "green"
  is_production = true
  is_default    = true
}

resource "watchpoint_environment" "staging" {
  key         = "staging"
  name        = "Staging"
  color       = "amber"
  reassign_to = watchpoint_environment.production.id
}

resource "watchpoint_http_monitor" "staging_api" {
  name           = "Staging API"
  url            = "https://staging.example.com/health"
  environment_id = watchpoint_environment.staging.id
}
