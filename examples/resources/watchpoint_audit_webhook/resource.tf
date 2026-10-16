resource "watchpoint_audit_webhook" "example" {
  name = "Splunk SOC"
  url  = "https://siem.example.com/ingest/watchpoint"
}
