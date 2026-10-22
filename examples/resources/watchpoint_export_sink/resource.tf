resource "watchpoint_export_sink" "lake" {
  name              = "Data lake"
  kind              = "s3"
  bucket            = "acme-watchpoint-export"
  region            = "us-east-1"
  datasets          = ["check_results", "monitor_hourly", "alert_events"]
  environment_ids   = [watchpoint_environment.production.id]
  schedule_hour_utc = 3
  secret            = "${var.aws_access_key_id}:${var.aws_secret_access_key}"
}

resource "watchpoint_export_sink" "archive" {
  name        = "Azure archive"
  kind        = "azure_blob"
  account_url = "https://acmearchive.blob.core.windows.net"
  container   = "watchpoint"
  format      = "csv"
  datasets    = ["monitors", "incidents"]
  secret      = var.archive_sas_token
}
