data "skycloak_realm_groups" "all" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
}
