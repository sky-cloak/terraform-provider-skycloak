data "skycloak_realm_users" "all" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
}
