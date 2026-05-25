resource "skycloak_domain" "app" {
  cluster_id = skycloak_cluster.production.id
  domain     = "auth.example.com"
}

# Create these DNS records at your provider to verify + route the domain.
output "dns_records" {
  value = skycloak_domain.app.dns_records
}
