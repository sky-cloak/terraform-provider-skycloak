resource "skycloak_realm_group" "engineering" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
  name       = "engineering"
}

resource "skycloak_realm_group" "backend" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name
  name       = "backend"
  parent_id  = skycloak_realm_group.engineering.id
}
