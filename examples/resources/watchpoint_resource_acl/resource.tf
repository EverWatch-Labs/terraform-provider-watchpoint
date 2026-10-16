# Pin the payments on-call role to the monitors it owns. Members of the role can
# open and list only these; every other kind (dashboards, status pages, RUM
# sites) is untouched, because each kind is narrowed independently.
resource "watchpoint_resource_acl" "payments_monitors" {
  resource_kind = "monitor"
  role_id       = watchpoint_role.payments_oncall.id
  label         = "Payments monitors"

  patterns = [
    watchpoint_http_monitor.checkout.id,
    watchpoint_http_monitor.refunds.id,
  ]
}

# A prefix glob covers a fleet without listing it. Use "*" to grant every
# resource of the kind, which is how you narrow one kind while leaving another
# wide open on purpose.
resource "watchpoint_resource_acl" "payments_dashboards" {
  resource_kind = "dashboard"
  role_id       = watchpoint_role.payments_oncall.id
  patterns      = ["dash_payments-*"]
}

# A grant can name a single member instead of a role. Exactly one of role_id or
# user_id is set.
resource "watchpoint_resource_acl" "contractor_status_page" {
  resource_kind = "status_page"
  user_id       = "usr_contractor"
  patterns      = [watchpoint_status_page.public.id]
}
