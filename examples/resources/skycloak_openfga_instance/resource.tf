resource "skycloak_realm" "orders" {
  cluster_id = skycloak_cluster.production.id
  name       = "orders"
}

# One OpenFGA instance per realm. Its API accepts tokens issued by that realm.
resource "skycloak_openfga_instance" "orders" {
  cluster_id = skycloak_cluster.production.id
  realm      = skycloak_realm.orders.name
}

output "openfga_api_url" {
  value = skycloak_openfga_instance.orders.api_url
}
