data "skycloak_cluster_credentials" "production" {
  cluster_id = skycloak_cluster.production.id
}
