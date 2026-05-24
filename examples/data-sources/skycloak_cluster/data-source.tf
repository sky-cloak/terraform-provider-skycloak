data "skycloak_cluster" "existing" {
  id = "d290f1ee-6c54-4b01-90e6-d701748f0851"
}

output "existing_status" {
  value = data.skycloak_cluster.existing.status
}
