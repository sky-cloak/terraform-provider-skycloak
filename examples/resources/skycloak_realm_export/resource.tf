resource "skycloak_realm_export" "customer_portal" {
  cluster_id = skycloak_cluster.production.id
  realm      = skycloak_realm.customer_portal.name

  # Always required: a realm export contains credentials, so the archive is
  # always encrypted.
  encryption_password = var.realm_export_password
}

output "realm_export_download_url" {
  # Presigned and short-lived: Skycloak deletes the archive 24h after the export
  # completes, after which this is null.
  value     = skycloak_realm_export.customer_portal.download_url
  sensitive = true
}

output "realm_export_source_version" {
  # An import target must run this Keycloak version or newer.
  value = skycloak_realm_export.customer_portal.source_version
}
