data "skycloak_cluster_locations" "all" {}

# Region codes are us (US East), us-west (US West), ca, eu and au. A region your
# workspace cannot deploy to yet is listed with available = false.
output "available_regions" {
  value = [for l in data.skycloak_cluster_locations.all.locations : l.location if l.available]
}
