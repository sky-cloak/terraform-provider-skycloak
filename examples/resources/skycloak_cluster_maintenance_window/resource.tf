resource "skycloak_cluster_maintenance_window" "production" {
  cluster_id   = skycloak_cluster.production.id
  enabled      = true
  days_of_week = [2, 4] # Tuesday and Thursday
  start_local  = "02:00"
  end_local    = "05:00"
  timezone     = "Europe/Berlin"
}

# Destroying this resource reverts the cluster to the workspace default
# window; a managed cluster always has an effective maintenance window.
