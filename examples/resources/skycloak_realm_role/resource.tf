resource "skycloak_realm_role" "admin" {
  cluster_id  = skycloak_cluster.production.id
  realm_name  = skycloak_realm.app.name
  name        = "admin"
  description = "Administrators"
}
