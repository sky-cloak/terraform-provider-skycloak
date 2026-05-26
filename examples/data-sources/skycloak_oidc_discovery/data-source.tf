data "skycloak_oidc_discovery" "google" {
  issuer_url = "https://accounts.google.com"
}

# Feed discovered endpoints into an identity provider.
resource "skycloak_identity_provider" "google" {
  cluster_id   = skycloak_cluster.production.id
  realm_name   = skycloak_realm.app.name
  provider_id  = "google"
  display_name = "Google"
  enabled      = true

  config = {
    oidc = {
      issuer    = data.skycloak_oidc_discovery.google.issuer
      token_url = data.skycloak_oidc_discovery.google.token_endpoint
    }
  }
}
