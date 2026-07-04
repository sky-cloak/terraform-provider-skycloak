data "skycloak_siem_destinations" "all" {}

output "unhealthy_destinations" {
  value = [for d in data.skycloak_siem_destinations.all.destinations : d.name if d.health_status != "healthy"]
}
