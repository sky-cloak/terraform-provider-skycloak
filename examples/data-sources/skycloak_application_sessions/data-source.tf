data "skycloak_application_sessions" "web" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
  client_id  = skycloak_application.web.client_id
}
