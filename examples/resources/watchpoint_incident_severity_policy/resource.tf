# Tighten the top of the ladder: a SEV1 must be picked up in two minutes, pages
# the primary on-call policy, and owes a postmortem within two days.
resource "watchpoint_incident_severity_policy" "sev1" {
  severity                  = "Sev1"
  response_target_minutes   = 2
  require_postmortem        = true
  postmortem_due_hours      = 48
  page_escalation_policy_id = watchpoint_escalation_policy.primary.id
}

# Rungs left alone keep their built-in defaults, so configure only what differs.
# Here SEV3 keeps its 60-minute target and simply starts owing a postmortem.
resource "watchpoint_incident_severity_policy" "sev3" {
  severity             = "Sev3"
  require_postmortem   = true
  postmortem_due_hours = 168
}
