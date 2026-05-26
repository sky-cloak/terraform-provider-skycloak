data "skycloak_cluster_events_export" "production" {
  cluster_id = skycloak_cluster.production.id
}
