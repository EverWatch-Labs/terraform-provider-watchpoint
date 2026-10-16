resource "watchpoint_dashboard_folder" "payments" {
  name = "Payments"
}

resource "watchpoint_dashboard_folder" "payments_oncall" {
  name      = "On-call views"
  parent_id = watchpoint_dashboard_folder.payments.id
}

resource "watchpoint_dashboard" "checkout" {
  name           = "Checkout health"
  folder_id      = watchpoint_dashboard_folder.payments_oncall.id
  owner_group_id = watchpoint_user_group.payments.id
}
