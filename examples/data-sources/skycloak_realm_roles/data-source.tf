data "skycloak_realm_roles" "all" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
}
