# DEPRECATED. Composite rules are no longer evaluated by the platform, so the
# rule below is stored and never fires. It still applies cleanly against the
# retained API so existing configurations keep working; migrate to the
# watchpoint_sgl_alert_rule underneath it.
resource "watchpoint_composite_alert_rule" "example" {
  name             = "Core services degraded"
  logic            = "Or"
  alert_profile_id = "ap_001"

  conditions = [
    { monitor_id = "mon_api", monitor_type = "HttpApi", fail_status = "Critical" },
    { monitor_id = "mon_db", monitor_type = "Tcp", fail_status = "Critical" },
  ]
}

# The same rule as an SGL alert rule. Every check result is written into the
# `logs` pillar carrying monitor.id and check.status, which is what makes the
# composite conditions expressible as one query: dcount over the monitor id is
# the count of distinct monitors that reported a failing check in the window.
# The summarize is deliberately ungrouped, so the rule opens ONE alert for the
# whole group the way a composite rule did, instead of one alert per monitor.
resource "watchpoint_sgl_alert_rule" "example_migrated" {
  name        = "Core services degraded"
  description = "1 of the 2 selected monitors reported a failing check within the last 5 minutes."

  sgl_query = <<-SGL
    logs
    | where attributes.check_status in ('critical', 'error', 'timeout') and attributes.monitor_id in ('mon_api', 'mon_db')
    | summarize failing=dcount(attributes.monitor_id)
  SGL

  # logic maps onto the threshold, not onto the query:
  #   logic = "Or"   -> threshold 1 (any one condition failing)
  #   logic = "And"  -> threshold = the number of monitors listed
  #   logic = "NOfM" -> threshold = min_fail_count
  operator  = "gte"
  threshold = 1

  # fail_status maps onto the check_status list. The list above is the failing
  # set; add 'warning' to it for conditions that used fail_status = "Warning".
  # monitor_type has no equivalent and needs none, because every check type
  # writes the same monitor.id and check.status attributes.
  severity = "Critical"

  # time_window_seconds -> time_range. 5m is the evaluator's shortest window, so
  # this counts monitors that failed anywhere inside the window rather than
  # monitors that are down at the same instant. cooldown_minutes has no
  # equivalent; interval_seconds is the evaluation cadence.
  time_range       = "5m"
  interval_seconds = 300

  alert_profile_id = "ap_001"
}
