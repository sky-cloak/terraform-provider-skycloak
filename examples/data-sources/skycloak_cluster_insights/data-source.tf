data "skycloak_cluster_insights" "overview" {
  cluster_id = skycloak_cluster.production.id
  type       = "overview"
}

output "insights" {
  value = jsondecode(data.skycloak_cluster_insights.overview.json)
}
