data "skycloak_themes" "all" {
  cluster_id = skycloak_cluster.production.id
}

output "deployed_theme_names" {
  value = [for t in data.skycloak_themes.all.themes : t.name if t.status == "deployed"]
}
