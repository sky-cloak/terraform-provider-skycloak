resource "skycloak_login_branding" "app" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name

  primary_color    = "#0ea5e9"
  background_color = "#0f172a"
  logo_url         = "https://cdn.example.com/logo.png"

  registration_enabled    = true
  forgot_password_enabled = true
  show_powered_by         = false

  internationalization = {
    enabled                    = true
    default_locale             = "en"
    supported_locales          = ["en", "fr"]
    language_selection_mode    = "automatic_with_selector"
    language_selector_position = "top_right"
    language_selector_style    = "dropdown"
  }

  sso = {
    enabled       = true
    button_size   = "medium"
    display_style = "logo_with_text"
    layout        = "horizontal"
  }
}
