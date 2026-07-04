package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccCAPTCHADomainResource exercises register → import → unregister against
// a pre-provisioned dev cluster (SKYCLOAK_ACCEPTANCE_CLUSTER_ID).
func TestAccCAPTCHADomainResource(t *testing.T) {
	clusterID := testAccClusterID(t)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "skycloak_captcha_domain" "test" {
  cluster_id = %q
  hostname   = "tf-acc.invalid.example.com"
}`, clusterID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skycloak_captcha_domain.test", "hostname", "tf-acc.invalid.example.com"),
					resource.TestCheckResourceAttrSet("skycloak_captcha_domain.test", "created_at"),
				),
			},
			{
				ResourceName:      "skycloak_captcha_domain.test",
				ImportState:       true,
				ImportStateId:     clusterID + "/tf-acc.invalid.example.com",
				ImportStateVerify: true,
			},
		},
	})
}
