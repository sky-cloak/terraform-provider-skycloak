resource "skycloak_cluster" "production" {
  name     = "production"
  type     = "keycloak"
  size     = "small"
  version  = "26.1"
  location = "eu"
}

output "cluster_url" {
  value = skycloak_cluster.production.url
}
