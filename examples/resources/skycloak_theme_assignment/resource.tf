# Look up an uploaded custom theme by name.
data "skycloak_themes" "all" {
  cluster_id = skycloak_cluster.production.id
}

locals {
  corporate_theme = one([for t in data.skycloak_themes.all.themes : t.id if t.name == "corporate"])
}

resource "skycloak_theme_assignment" "app" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name

  login = local.corporate_theme
  # account, admin, email omitted -> Keycloak built-in defaults
}
