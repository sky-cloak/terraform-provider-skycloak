resource "skycloak_realm_user" "jdoe" {
  cluster_id         = skycloak_cluster.production.id
  realm_name         = skycloak_realm.app.name
  username           = "jdoe"
  email              = "jdoe@example.com"
  first_name         = "Jane"
  last_name          = "Doe"
  enabled            = true
  temporary_password = var.jdoe_password
}
