resource "skycloak_identity_provider" "google" {
  cluster_id   = skycloak_cluster.production.id
  realm_name   = skycloak_realm.app.name
  provider_id  = "google"
  type         = "oidc"
  display_name = "Sign in with Google"
  enabled      = true

  client_id     = "your-google-client-id"
  client_secret = var.google_client_secret

  config = {
    button_text = "Sign in with Google"

    oidc = {
      issuer            = "https://accounts.google.com"
      authorization_url = "https://accounts.google.com/o/oauth2/v2/auth"
      token_url         = "https://oauth2.googleapis.com/token"
    }

    attribute_mappings = {
      email = "email"
    }
  }
}
