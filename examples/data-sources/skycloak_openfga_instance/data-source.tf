# Look up an instance by the realm it is bound to...
data "skycloak_openfga_instance" "orders" {
  cluster_id = skycloak_cluster.production.id
  realm      = "orders"
}

# ...or by its ID.
data "skycloak_openfga_instance" "by_id" {
  cluster_id = skycloak_cluster.production.id
  id         = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
}
