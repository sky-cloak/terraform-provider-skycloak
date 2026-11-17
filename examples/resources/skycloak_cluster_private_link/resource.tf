# Requires an Enterprise plan and a cluster on a site with Azure private link.
resource "skycloak_cluster_private_link" "example" {
  cluster_id             = skycloak_cluster.example.id
  enabled                = true
  azure_subscription_ids = ["11111111-1111-1111-1111-111111111111"]
}

output "private_link_alias" {
  value = skycloak_cluster_private_link.example.alias
}

# Restore public access before destroying this resource if it is off.
