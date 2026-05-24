resource "skycloak_identity_provider" "google" {
  cluster_id   = skycloak_cluster.production.id
  realm_name   = skycloak_realm.app.name
  provider_id  = "google"
  type         = "oidc"
  display_name = "Sign in with Google"
  enabled      = true

  config = {
    clientId         = "your-google-client-id"
    clientSecret     = "your-google-client-secret"
    authorizationUrl = "https://accounts.google.com/o/oauth2/v2/auth"
    tokenUrl         = "https://oauth2.googleapis.com/token"
  }
}
