resource "skycloak_cluster_features" "example" {
  cluster_id = skycloak_cluster.example.id
  features   = ["cimd"]

  # Use the maintenance window by default.
  # Set apply_now = true to restart immediately and wait for Keycloak.
}
