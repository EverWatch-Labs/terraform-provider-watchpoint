# Parse checkout's access lines, drop debug noise, and mask an internal
# account-number format. Declare this resource once per tenant.
resource "watchpoint_log_pipeline" "this" {
  processors_json = jsonencode([
    {
      type    = "grok"
      name    = "access line"
      enabled = true
      filter  = "service:checkout"
      source  = "body"
      pattern = "%%{IP:client.ip} %%{WORD:http.method} %%{URIPATH:url.path} %%{NUMBER:http.status_code}"
    },
    {
      type    = "drop"
      name    = "debug noise"
      enabled = true
      filter  = "severity:debug"
    },
  ])

  pii_rules_json = jsonencode([
    {
      name            = "account_number"
      pattern         = "ACCT-[0-9]{8}"
      replacement     = "ACCT-[REDACTED]"
      appliesToFields = "body"
      isActive        = true
    },
  ])
}
