resource "watchpoint_http_monitor" "example" {
  name                   = "Production API"
  url                    = "https://api.example.com/health"
  method                 = "GET"
  check_interval_seconds = 60
  timeout_seconds        = 30
  expected_status_codes  = "200"
  expected_body_contains = "\"status\":\"ok\""
  tags                   = ["production", "public-api"]
}
