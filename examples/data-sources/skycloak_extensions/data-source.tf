data "skycloak_extensions" "all" {}

output "marketplace_extensions" {
  value = [for e in data.skycloak_extensions.all.extensions : e.name if e.source == "marketplace"]
}
