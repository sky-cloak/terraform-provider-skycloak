resource "skycloak_theme" "corporate" {
  cluster_id = skycloak_cluster.production.id
  source     = "${path.module}/themes/corporate.zip" # .zip or Keycloakify .jar
  name       = "corporate"
  version    = "1.2.0"

  # Optional: deploy only specific theme types (omit to deploy all detected).
  theme_types = ["login", "email"]
}

# Activate it on a realm.
resource "skycloak_theme_assignment" "app" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
  login      = skycloak_theme.corporate.id
}
