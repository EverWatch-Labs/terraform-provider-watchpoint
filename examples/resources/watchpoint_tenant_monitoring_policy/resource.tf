resource "watchpoint_tenant_monitoring_policy" "this" {
  lock_terraform_managed   = true
  approval_environment_ids = [watchpoint_environment.production.id]
  approval_min_approvers   = 2
  approval_allow_self      = false
}
