# Every HTTP monitor for the tenant.
data "watchpoint_monitors" "http" {
  type = "HttpApi"
}

output "http_monitor_names" {
  value = [for m in data.watchpoint_monitors.http.monitors : m.name]
}
