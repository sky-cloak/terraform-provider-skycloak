# Requires an Enterprise plan. Turning public access off is refused unless
# Azure private link is enabled and ready, so depend on the private link.
resource "skycloak_cluster_public_access" "example" {
  cluster_id    = skycloak_cluster.example.id
  public_access = false

  depends_on = [skycloak_cluster_private_link.example]
}
