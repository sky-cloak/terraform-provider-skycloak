resource "skycloak_cluster" "production" {
  name     = "production"
  type     = "keycloak"
  size     = "small"
  version  = "26.1"
  location = "eu"
}

resource "skycloak_realm" "app" {
  cluster_id               = skycloak_cluster.production.id
  name                     = "app"
  display_name             = "Application Realm"
  enabled                  = true
  login_with_email_allowed = true
}
