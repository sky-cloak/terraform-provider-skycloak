# Grant the realm-management "manage-users" client role to an app's service account.
resource "skycloak_application_role_assignment" "web_manage_users" {
  cluster_id     = skycloak_cluster.production.id
  realm_name     = skycloak_realm.app.name
  client_id      = skycloak_application.web.client_id
  role_name      = "manage-users"
  role_client_id = "realm-management"
}
