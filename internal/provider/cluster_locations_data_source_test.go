package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccClusterLocationsDataSource is a fast, read-only acceptance test: it
// validates authentication, endpoint, and provider wiring against the live API
// without creating any resources. Good first smoke test for a new environment.
func TestAccClusterLocationsDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "skycloak_cluster_locations" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.skycloak_cluster_locations.all", "locations.#"),
				),
			},
		},
	})
}
