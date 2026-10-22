resource "watchpoint_report_share_link" "exec" {
  report_id       = watchpoint_scheduled_report.weekly_sla.id
  label           = "Executive summary"
  expires_in_days = 90
}

output "exec_report_url" {
  value     = watchpoint_report_share_link.exec.url
  sensitive = true
}
