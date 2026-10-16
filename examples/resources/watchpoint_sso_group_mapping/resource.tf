resource "watchpoint_sso_group_mapping" "example" {
  config_id       = "sso_001"
  sso_group_claim = "Engineering"
  group_id        = "grp_001"
}
