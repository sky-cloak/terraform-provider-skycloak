package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccVerifiedDomain returns the hostname of one of the cluster's verified
// custom domains; the CAPTCHA domains API only accepts those. Skips when the
// cluster has none.
func testAccVerifiedDomain(t *testing.T, clusterID string) string {
	t.Helper()
	if os.Getenv("SKYCLOAK_API_KEY") == "" {
		t.Skip("SKYCLOAK_API_KEY not set; skipping acceptance test")
	}
	domains, err := testAccClient().ListDomains(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("listing domains to probe for a verified custom domain: %v", err)
	}
	for _, d := range domains {
		if d.VerificationStatus == "verified" {
			return d.Domain
		}
	}
	t.Skip("cluster has no verified custom domain; skipping CAPTCHA domain acceptance test")
	return ""
}

// TestAccCAPTCHADomainResource exercises register → import → unregister against
// a pre-provisioned dev cluster (SKYCLOAK_ACCEPTANCE_CLUSTER_ID). The API only
// registers hostnames that are verified custom domains of the cluster, so the
// test adopts one of the cluster's verified domains.
func TestAccCAPTCHADomainResource(t *testing.T) {
	clusterID := testAccClusterID(t)
	hostname := testAccVerifiedDomain(t, clusterID)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "skycloak_captcha_domain" "test" {
  cluster_id = %q
  hostname   = %q
}`, clusterID, hostname),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skycloak_captcha_domain.test", "hostname", hostname),
					resource.TestCheckResourceAttrSet("skycloak_captcha_domain.test", "created_at"),
				),
			},
			{
				ResourceName:      "skycloak_captcha_domain.test",
				ImportState:       true,
				ImportStateId:     clusterID + "/" + hostname,
				ImportStateVerify: true,
			},
		},
	})
}
