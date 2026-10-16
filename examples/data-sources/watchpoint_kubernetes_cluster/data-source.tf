# The cluster monitor created when the operator was installed with
# clusterName=prod-eu, filed under the platform team.
data "watchpoint_kubernetes_cluster" "prod_eu" {
  cluster_name = "prod-eu"
}

resource "watchpoint_monitor_policy" "prod_eu" {
  monitor_id     = data.watchpoint_kubernetes_cluster.prod_eu.id
  owner_group_id = watchpoint_user_group.platform.id
}
