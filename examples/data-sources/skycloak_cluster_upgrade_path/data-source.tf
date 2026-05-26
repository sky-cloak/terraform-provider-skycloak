data "skycloak_cluster_upgrade_path" "production" {
  cluster_id = skycloak_cluster.production.id
}
