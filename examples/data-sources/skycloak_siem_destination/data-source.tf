data "skycloak_siem_destination" "corporate" {
  id = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
}

output "siem_health" {
  value = data.skycloak_siem_destination.corporate.health_status
}
