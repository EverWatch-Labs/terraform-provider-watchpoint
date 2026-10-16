# Pull CloudWatch metrics for RDS and EC2 by assuming a role that trusts the
# external id watchpoint-<tenant id>.
resource "watchpoint_cloud_integration" "aws_prod" {
  provider_type  = "aws"
  name           = "AWS production"
  resource_types = ["rds", "ec2"]
  regions        = ["us-east-1", "eu-west-1"]
  role_arn       = "arn:aws:iam::123456789012:role/watchpoint-metrics"
}

# Azure Monitor through an app registration with Monitoring Reader.
resource "watchpoint_cloud_integration" "azure" {
  provider_type   = "azure"
  resource_types  = ["vm", "sql"]
  directory_id    = var.azure_tenant_id
  client_id       = var.azure_client_id
  client_secret   = var.azure_client_secret
  subscription_id = var.azure_subscription_id
}
