variable "healthcheck_token" {
  type        = string
  sensitive   = true
  description = "Bearer token stored in the Watchpoint vault and used by the API monitor."
  default     = "changeme"
}

variable "tenant_slug" {
  type        = string
  description = "Tenant slug used in the public status page URL ({slug}.status.everwatch-labs.io)."
  default     = "acme"
}

variable "release_version" {
  type        = string
  description = "Release identifier recorded as a deployment marker."
  default     = "v1.0.0"
}
