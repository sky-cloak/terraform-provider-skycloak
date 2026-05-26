data "skycloak_cluster_build" "latest" {
  cluster_id = skycloak_cluster.production.id
  build_id   = data.skycloak_cluster_builds.production.builds[0].id
}
