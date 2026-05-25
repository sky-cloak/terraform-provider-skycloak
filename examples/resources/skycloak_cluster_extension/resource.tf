# Look up a marketplace extension by name.
data "skycloak_extensions" "all" {}

locals {
  magic_link_id = one([for e in data.skycloak_extensions.all.extensions : e.id if e.name == "Magic Link"])
}

resource "skycloak_cluster_extension" "magic_link" {
  cluster_id   = skycloak_cluster.production.id
  extension_id = local.magic_link_id

  parameters = {
    sender_email = "no-reply@example.com"
  }
}
