resource "skycloak_domain_route" "app" {
  cluster_id           = skycloak_cluster.production.id
  domain_id            = skycloak_domain.app.id
  realm                = skycloak_realm.app.name
  hide_realm_path      = true
  cors_allowed_origins = ["https://app.example.com"]
}
