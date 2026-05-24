resource "skycloak_smtp" "app" {
  cluster_id = skycloak_cluster.production.id
  realm_name = skycloak_realm.app.name

  host       = "smtp.example.com"
  port       = 587
  encryption = "starttls"
  from_email = "no-reply@example.com"
  from_name  = "Example App"

  auth_type = "basic"
  username  = "smtp-user"
  password  = var.smtp_password # write-only; omit on later applies to keep it
}
