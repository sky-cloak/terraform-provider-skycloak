resource "skycloak_client_theme_assignment" "web" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
  client_id  = skycloak_application.web.client_id

  # Override just this client's login theme; omit to use the realm default.
  login = local.corporate_theme
}
