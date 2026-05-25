resource "skycloak_export" "nightly" {
  cluster_id = skycloak_cluster.production.id
  format     = "pgdump"

  # Including credentials requires an encryption password.
  include_credentials = true
  encryption_password = var.export_password
}

output "export_download_url" {
  value     = skycloak_export.nightly.download_url
  sensitive = true
}
