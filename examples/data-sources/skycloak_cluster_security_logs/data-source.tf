data "skycloak_cluster_security_logs" "recent" {
  cluster_id = skycloak_cluster.production.id
  limit      = 100
}
