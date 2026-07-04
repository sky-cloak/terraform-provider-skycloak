resource "skycloak_webhook_subscription" "ops" {
  name           = "ops-alerts"
  url            = "https://hooks.example.com/skycloak"
  source         = "keycloak"
  event_types    = ["LOGIN_ERROR", "USER_DISABLED_BY_PERMANENT_LOCKOUT"]
  signing_secret = var.webhook_signing_secret # write-only; never read back

  # Optional: scope deliveries to one cluster.
  cluster_id = skycloak_cluster.production.id
}
