# Archive closed days of logs to S3 as gzip JSONL. Declare once per tenant.
resource "watchpoint_log_archive_destination" "this" {
  type              = "s3"
  bucket            = "acme-watchpoint-logs"
  region            = "us-east-1"
  access_key_id     = var.log_archive_access_key_id
  secret_access_key = var.log_archive_secret_access_key
}
