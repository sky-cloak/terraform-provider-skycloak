package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccAPIKeyOnlyPlansNoChanges covers upgrades from releases that only
// had api_key: a configuration that sets that one provider argument, and none
// of the Platform ones, applies and then plans no changes.
func TestAccAPIKeyOnlyPlansNoChanges(t *testing.T) {
	t.Setenv("SKYCLOAK_PLATFORM_API_KEY", "")
	t.Setenv("SKYCLOAK_PLATFORM_ENDPOINT", "")
	config := fmt.Sprintf(`
provider "skycloak" {
  api_key = %q
}

data "skycloak_cluster_locations" "all" {}
`, os.Getenv("SKYCLOAK_API_KEY"))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttrSet("data.skycloak_cluster_locations.all", "locations.#"),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
