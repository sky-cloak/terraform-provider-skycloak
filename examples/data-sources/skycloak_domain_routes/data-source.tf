data "skycloak_domain_routes" "app" {
  cluster_id = skycloak_cluster.production.id
  domain_id  = skycloak_domain.app.id
}
