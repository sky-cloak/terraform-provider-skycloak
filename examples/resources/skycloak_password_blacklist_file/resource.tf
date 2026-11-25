# Upload a list of passwords to refuse, one per line (up to 25 MB, 10 files per
# cluster). The cluster must be dedicated.
resource "skycloak_password_blacklist_file" "common" {
  cluster_id = skycloak_cluster.production.id
  name       = "common-passwords-2026"
  source     = "${path.module}/blacklists/common-passwords.txt"

  # The first file on a cluster adds a small persistent folder to the Keycloak
  # pods, which takes a restart. By default it waits for the cluster's
  # maintenance window. Set apply_now to restart at once and wait until the
  # file is ready.
  apply_now = true

  # Files cannot be changed after upload: new content needs a new name. A realm
  # password policy that names the old file blocks its deletion, so create the
  # new file first, move the policy to it, then remove the old one.
  lifecycle {
    create_before_destroy = true
  }
}

# Once the file is ready, set the realm password policy
# passwordBlacklist(common-passwords-2026) on each realm that should use it.
