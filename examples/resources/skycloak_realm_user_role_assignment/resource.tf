resource "skycloak_realm_user_role_assignment" "jdoe_admin" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
  user_id    = skycloak_realm_user.jdoe.id
  role_name  = skycloak_realm_role.admin.name
}
