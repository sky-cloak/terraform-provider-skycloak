data "skycloak_webhook_event_types" "all" {}

output "keycloak_event_types" {
  value = [for e in data.skycloak_webhook_event_types.all.event_types : e.type if e.category == "keycloak" && !e.deprecated]
}
