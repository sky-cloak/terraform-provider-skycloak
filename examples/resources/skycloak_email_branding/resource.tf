resource "skycloak_email_branding" "app" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name

  primary_color         = "#111827"
  header_logo_light_url = "https://cdn.example.com/logo-light.png"
  header_logo_dark_url  = "https://cdn.example.com/logo-dark.png"
  footer_company_name   = "Example, Inc."
  footer_text           = "You received this email because you have an account with us."
  company_url           = "https://example.com"

  internationalization = {
    enabled           = true
    default_locale    = "en"
    supported_locales = ["en", "de"]
  }
}
