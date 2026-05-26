data "skycloak_cluster_logs" "errors" {
  cluster_id = skycloak_cluster.production.id
  level      = "ERROR"
  limit      = 100
}
