# Import a realm from a local artifact. The file is staged to object storage
# through a presigned URL, then imported.
resource "skycloak_realm_import" "from_file" {
  cluster_id  = skycloak_cluster.production.id
  source_file = "${path.module}/customer-portal.zip.enc"
  password    = var.realm_artifact_password
}

# Or import from an export that is still stored in Skycloak, with no upload.
# The target cluster must not already have a realm of the same name.
resource "skycloak_realm_import" "from_export" {
  cluster_id       = skycloak_cluster.staging.id
  source_kind      = "stored"
  source_export_id = skycloak_realm_export.customer_portal.id
  password         = var.realm_export_password
}

output "imported_realm" {
  value = skycloak_realm_import.from_file.realm
}
