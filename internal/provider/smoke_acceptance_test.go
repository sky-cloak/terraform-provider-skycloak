package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSmoke* are fast, read-only acceptance tests: they validate auth,
// endpoint, API-Version pinning, and provider wiring against the live API
// without creating anything. The daily scheduled CI lane runs exactly this
// set (test.yml filters on the TestAccSmoke prefix).

func TestAccSmokeClusterLocations(t *testing.T) {
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

func TestAccSmokeClusterTypes(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "skycloak_cluster_types" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.skycloak_cluster_types.all", "types.#"),
				),
			},
		},
	})
}

func TestAccSmokeClusterFeatures(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "skycloak_cluster_features" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.skycloak_cluster_features.all", "features.#"),
				),
			},
		},
	})
}

// TestAccSmokeClusterVersions also guards the shape of the cluster-type
// versions payload, which changed from bare strings to structured objects.
func TestAccSmokeClusterVersions(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "skycloak_cluster_versions" "keycloak" { type = "keycloak" }`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.skycloak_cluster_versions.keycloak", "versions.#"),
					resource.TestCheckResourceAttrSet("data.skycloak_cluster_versions.keycloak", "version_details.#"),
					resource.TestCheckResourceAttrSet("data.skycloak_cluster_versions.keycloak", "version_details.0.version"),
					resource.TestCheckResourceAttrSet("data.skycloak_cluster_versions.keycloak", "version_details.0.active"),
					resource.TestCheckResourceAttrSet("data.skycloak_cluster_versions.keycloak", "version_details.0.breaking_change_count"),
				),
			},
		},
	})
}

func TestAccSmokeWebhookEventTypes(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "skycloak_webhook_event_types" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.skycloak_webhook_event_types.all", "event_types.#"),
				),
			},
		},
	})
}
