resource "skycloak_application" "web" {
  cluster_id    = skycloak_cluster.production.id
  realm_name    = skycloak_realm.app.name
  client_id     = "web-app"
  name          = "Web App"
  type          = "confidential"
  protocol      = "openid-connect"
  redirect_uris = ["https://app.example.com/callback"]
  pkce_required = true
}

# The generated secret is sensitive and available after apply.
output "web_client_secret" {
  value     = skycloak_application.web.client_secret
  sensitive = true
}
