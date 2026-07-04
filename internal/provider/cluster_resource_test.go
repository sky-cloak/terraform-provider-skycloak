package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// testAccProtoV6ProviderFactories wires the provider for acceptance tests.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"skycloak": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	if os.Getenv("SKYCLOAK_API_KEY") == "" {
		t.Skip("SKYCLOAK_API_KEY not set; skipping acceptance test")
	}
}

// testAccClient returns a facade client built from the acceptance env vars,
// for probes that decide whether a test can run at all.
func testAccClient() *skycloak.Client {
	return skycloak.New(os.Getenv("SKYCLOAK_ENDPOINT"), os.Getenv("SKYCLOAK_API_KEY"), os.Getenv("SKYCLOAK_API_VERSION"))
}

// testAccClusterID returns the pre-provisioned cluster acceptance tests that
// need an existing cluster should target, skipping when none is configured.
// Provisioning a cluster per test run is disproportionate for sub-resource
// tests; SKYCLOAK_ACCEPTANCE_CLUSTER_ID points them at a long-lived dev
// cluster instead.
func testAccClusterID(t *testing.T) string {
	t.Helper()
	id := os.Getenv("SKYCLOAK_ACCEPTANCE_CLUSTER_ID")
	if id == "" {
		t.Skip("SKYCLOAK_ACCEPTANCE_CLUSTER_ID not set; skipping cluster-scoped acceptance test")
	}
	return id
}

// TestAccClusterResource exercises create → read → import → destroy against a
// real dev workspace. Runs only with TF_ACC=1 and a SKYCLOAK_API_KEY set;
// otherwise resource.Test skips it.
func TestAccClusterResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "skycloak_cluster" "test" {
  name     = "tf-acc-test"
  type     = "keycloak"
  size     = "small"
  version  = "26.1"
  location = "eu"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("skycloak_cluster.test", "id"),
					resource.TestCheckResourceAttr("skycloak_cluster.test", "status", "available"),
				),
			},
			{
				ResourceName:      "skycloak_cluster.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
