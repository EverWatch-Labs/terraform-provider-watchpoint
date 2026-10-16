resource "watchpoint_deployment" "example" {
  version     = "v2.3.1"
  service     = "api-server"
  environment = "production"
  monitor_ids = ["mon_abc123"]
}
