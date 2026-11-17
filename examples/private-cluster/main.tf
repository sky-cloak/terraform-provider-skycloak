# A private cluster, end to end: a cluster on a site with Azure private link,
# public access turned off, a realm behind your own proxy, and a custom theme.
# Private link and turning public access off need an Enterprise plan.
terraform {
  required_providers {
    skycloak = {
      source  = "sky-cloak/skycloak"
      version = "~> 0.1"
    }
  }
}

provider "skycloak" {
  # api_key is read from the SKYCLOAK_API_KEY environment variable.
}

# The site must support Azure private link and be open to your workspace.
resource "skycloak_cluster" "private" {
  name     = "private"
  type     = "keycloak"
  size     = "small"
  version  = "26.1"
  location = "eu"
  site     = "azure-north-europe"
}

resource "skycloak_cluster_private_link" "private" {
  cluster_id             = skycloak_cluster.private.id
  enabled                = true
  azure_subscription_ids = ["11111111-1111-1111-1111-111111111111"]
}

# Public access can only be turned off once the private link is enabled and
# ready, so it depends on the private link. On destroy, public access is turned
# back on before the private link is disabled.
resource "skycloak_cluster_public_access" "private" {
  cluster_id    = skycloak_cluster.private.id
  public_access = false

  depends_on = [skycloak_cluster_private_link.private]
}

resource "skycloak_realm" "app" {
  cluster_id   = skycloak_cluster.private.id
  name         = "app"
  display_name = "Application Realm"
  enabled      = true

  # The public URL the realm serves on, behind your own proxy.
  frontend_url = "https://login.example.com"
}

# You must supply the theme JAR: build your own Keycloakify theme and place it
# at themes/corporate.jar next to this file, or change the source path.
resource "skycloak_custom_theme" "corporate" {
  cluster_id = skycloak_cluster.private.id
  name       = "corporate"
  version    = "1.0.0"
  source     = "${path.module}/themes/corporate.jar" # Keycloakify .jar
}

resource "skycloak_theme_assignment" "app" {
  cluster_id = skycloak_cluster.private.id
  realm_name = skycloak_realm.app.name
  login      = skycloak_custom_theme.corporate.id
}

output "private_link_alias" {
  value = skycloak_cluster_private_link.private.alias
}
