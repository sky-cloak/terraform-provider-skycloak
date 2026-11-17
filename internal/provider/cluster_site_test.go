package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func clusterSiteConfig(endpoint, site string) string {
	siteLine := ""
	if site != "" {
		siteLine = fmt.Sprintf("site     = %q", site)
	}
	return fmt.Sprintf(`
provider "skycloak" {
  endpoint = %q
  api_key  = "sk_sc_test_aaa_bbb"
}

resource "skycloak_cluster" "test" {
  name     = "tf-location"
  type     = "keycloak"
  size     = "small"
  version  = "26.1"
  location = "eu"
  %s
}

data "skycloak_cluster" "test" {
  id = skycloak_cluster.test.id
}
`, endpoint, siteLine)
}

// TestClusterSiteSent: a configured site is sent on create, read back into
// state and exposed by the data source.
func TestClusterSiteSent(t *testing.T) {
	_, srv := newClusterLocationAPI(t, "eu", false)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: clusterSiteConfig(srv.URL, "azure-north-europe"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(clusterAddr, "site", "azure-north-europe"),
					resource.TestCheckResourceAttr("data.skycloak_cluster.test", "site", "azure-north-europe"),
				),
			},
			{
				Config: clusterSiteConfig(srv.URL, "azure-north-europe"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestClusterSiteOmitted: leaving the site out is accepted, plans no diff on
// later runs, and an API that reports no site leaves it null.
func TestClusterSiteOmitted(t *testing.T) {
	api, srv := newClusterLocationAPI(t, "eu", false)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: clusterSiteConfig(srv.URL, ""),
				Check:  resource.TestCheckNoResourceAttr(clusterAddr, "site"),
			},
			{
				// The API now reports the default site the cluster landed on.
				PreConfig: func() { api.reportSite("azure-north-europe") },
				Config:    clusterSiteConfig(srv.URL, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(clusterAddr, "site", "azure-north-europe"),
			},
		},
	})
}

// TestClusterSiteChangeReplaces: changing a configured site replaces the cluster.
func TestClusterSiteChangeReplaces(t *testing.T) {
	_, srv := newClusterLocationAPI(t, "eu", false)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: clusterSiteConfig(srv.URL, "site-a")},
			{
				Config: clusterSiteConfig(srv.URL, "site-b"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(clusterAddr, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.TestCheckResourceAttr(clusterAddr, "site", "site-b"),
			},
		},
	})
}
