terraform {
  required_providers {
    watchpoint = {
      source = "everwatch-labs/watchpoint"
    }
  }
}

# Credentials are best supplied via environment variables:
#   WATCHPOINT_API_KEY, WATCHPOINT_TENANT_ID
provider "watchpoint" {
  api_key   = var.watchpoint_api_key
  tenant_id = var.watchpoint_tenant_id

  # Recorded on every monitor this configuration manages. Defaults to
  # TF_WORKSPACE, else "default".
  workspace = "production"
}

variable "watchpoint_api_key" {
  type      = string
  sensitive = true
}

variable "watchpoint_tenant_id" {
  type = string
}
