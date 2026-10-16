# Deploy markers for the checkout service in a pinned window.
data "watchpoint_annotations" "checkout_deploys" {
  from     = "2026-10-01T00:00:00Z"
  to       = "2026-10-08T00:00:00Z"
  source   = "deploy"
  ref_kind = "service"
  ref_id   = "checkout"
}

output "checkout_deploy_labels" {
  value = [for a in data.watchpoint_annotations.checkout_deploys.annotations : a.label]
}
