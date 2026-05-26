data "skycloak_cluster_upgrades" "production" {
  cluster_id = skycloak_cluster.production.id
}
