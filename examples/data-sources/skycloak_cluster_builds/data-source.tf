data "skycloak_cluster_builds" "production" {
  cluster_id = skycloak_cluster.production.id
}
