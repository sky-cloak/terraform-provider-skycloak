resource "skycloak_application_secret" "web" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
  client_id  = skycloak_application.web.client_id

  # Change any value here to force a new client secret to be generated.
  rotate_when = {
    rotated_on = "2026-01-01"
  }
}

output "web_rotated_secret" {
  value     = skycloak_application_secret.web.client_secret
  sensitive = true
}
