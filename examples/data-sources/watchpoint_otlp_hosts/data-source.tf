# Hosts the tenant's OTLP host metrics have been seen from in the last 24 hours.
data "watchpoint_otlp_hosts" "fleet" {}

output "host_names" {
  value = [for h in data.watchpoint_otlp_hosts.fleet.hosts : h.host_name]
}

# A wider, pinned window. The list is a rolling observation, so a host that stops
# reporting drops out of it: anything built with for_each over these hosts is
# destroyed when that happens.
data "watchpoint_otlp_hosts" "last_week" {
  from = "2026-09-10T00:00:00Z"
  to   = "2026-09-17T00:00:00Z"
}
