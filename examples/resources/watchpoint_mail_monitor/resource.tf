# Check that the SMTP relay answers, negotiates STARTTLS and accepts a login.
resource "watchpoint_mail_monitor" "relay" {
  name                     = "SMTP relay"
  host                     = "smtp.example.com"
  port                     = 587
  protocol                 = "Smtp"
  tls_mode                 = "StartTls"
  username                 = "monitor@example.com"
  password                 = "{{vault.SMTP_MONITOR_PASSWORD}}"
  expected_banner_contains = "ESMTP"
  max_response_time_ms     = 3000
}
