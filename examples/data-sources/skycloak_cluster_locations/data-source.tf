data "skycloak_cluster_locations" "all" {}

output "available_regions" {
  value = [for l in data.skycloak_cluster_locations.all.locations : l.location if l.available]
}
