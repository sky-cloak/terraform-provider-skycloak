package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccClusterMaintenanceWindowResource exercises upsert → update → import →
// destroy against a pre-provisioned dev cluster (SKYCLOAK_ACCEPTANCE_CLUSTER_ID).
// Destroy reverts the cluster to the workspace default window server-side.
func TestAccClusterMaintenanceWindowResource(t *testing.T) {
	clusterID := testAccClusterID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "skycloak_cluster_maintenance_window" "test" {
  cluster_id   = %q
  enabled      = true
  days_of_week = [2, 4]
  start_local  = "02:00"
  end_local    = "05:00"
  timezone     = "UTC"
}`, clusterID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skycloak_cluster_maintenance_window.test", "enabled", "true"),
					resource.TestCheckResourceAttr("skycloak_cluster_maintenance_window.test", "days_of_week.#", "2"),
					resource.TestCheckResourceAttr("skycloak_cluster_maintenance_window.test", "start_local", "02:00"),
				),
			},
			{
				Config: fmt.Sprintf(`
resource "skycloak_cluster_maintenance_window" "test" {
  cluster_id   = %q
  enabled      = true
  days_of_week = [0, 6]
  start_local  = "01:30"
  end_local    = "04:30"
  timezone     = "America/Toronto"
}`, clusterID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skycloak_cluster_maintenance_window.test", "timezone", "America/Toronto"),
					resource.TestCheckResourceAttr("skycloak_cluster_maintenance_window.test", "start_local", "01:30"),
				),
			},
			{
				ResourceName:      "skycloak_cluster_maintenance_window.test",
				ImportState:       true,
				ImportStateId:     clusterID,
				ImportStateVerify: true,
			},
		},
	})
}
