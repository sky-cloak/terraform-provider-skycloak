data "skycloak_cluster_versions" "keycloak" {
  type = "keycloak"
}

output "latest_keycloak_version" {
  value = data.skycloak_cluster_versions.keycloak.versions[0]
}
