resource "skycloak_cluster" "production" {
  name     = "production"
  type     = "keycloak"
  size     = "small"
  version  = "26.1"
  # One of us (US East), us-west (US West), ca, eu, au.
  location = "eu"

  # Optional: a site of the location, such as one with Azure private link.
  # Leave it out for the location's default site. Changing it replaces the cluster.
  # site = "azure-north-europe"
}

output "cluster_url" {
  value = skycloak_cluster.production.url
}
