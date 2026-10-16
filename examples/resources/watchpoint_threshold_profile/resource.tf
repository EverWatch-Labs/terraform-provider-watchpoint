resource "watchpoint_threshold_profile" "example" {
  name = "Strict"
  rules = [
    {
      metric               = "ResponseTimeMs"
      operator             = "GreaterThan"
      warning_value        = 2000
      critical_value       = 5000
      consecutive_breaches = 2
    },
  ]
}
