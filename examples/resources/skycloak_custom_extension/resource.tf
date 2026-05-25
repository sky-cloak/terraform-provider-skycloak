resource "skycloak_custom_extension" "magic_link" {
  jar              = "${path.module}/extensions/magic-link-1.0.0.jar"
  name             = "Magic Link"
  keycloak_version = "26"
  version          = "1.0.0" # bump together with the jar to publish a new version
  description      = "Passwordless email login"

  parameter_type = "global"
  parameters = [
    {
      key      = "sender_email"
      label    = "Sender email"
      type     = "text"
      required = true
    },
    {
      key          = "api_key"
      label        = "Provider API key"
      type         = "password"
      required     = true
      is_sensitive = true
    },
  ]
}

# Install the uploaded extension on a cluster.
resource "skycloak_cluster_extension" "magic_link" {
  cluster_id   = skycloak_cluster.production.id
  extension_id = skycloak_custom_extension.magic_link.id

  parameters = {
    sender_email = "no-reply@example.com"
    api_key      = var.magic_link_api_key
  }
}
