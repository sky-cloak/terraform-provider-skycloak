resource "skycloak_realm_group_membership" "jdoe_eng" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
  user_id    = skycloak_realm_user.jdoe.id
  group_id   = skycloak_realm_group.engineering.id
}
