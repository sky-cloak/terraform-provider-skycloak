data "skycloak_cluster_events" "logins" {
  cluster_id = skycloak_cluster.production.id
  category   = "user"
  realm      = skycloak_realm.app.name
}
