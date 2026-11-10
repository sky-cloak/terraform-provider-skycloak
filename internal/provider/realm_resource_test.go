package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

func testAccRealmDestroyed(clusterID string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "skycloak_realm" {
				continue
			}
			_, err := testAccClient().GetRealm(context.Background(), clusterID, rs.Primary.Attributes["name"])
			if err == nil {
				return fmt.Errorf("realm %s still exists", rs.Primary.ID)
			}
			if !skycloak.IsNotFound(err) {
				return fmt.Errorf("checking realm %s: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

// TestAccRealmResourceFrontendURL exercises create with a frontend URL, update,
// clear, set again and import against a pre-provisioned dedicated dev cluster
// (SKYCLOAK_ACCEPTANCE_CLUSTER_ID). Setting a frontend URL is refused on
// clusters that are not dedicated.
func TestAccRealmResourceFrontendURL(t *testing.T) {
	clusterID := testAccClusterID(t)
	const res = "skycloak_realm.test"
	config := func(frontendURL string) string {
		attr := ""
		if frontendURL != "" {
			attr = fmt.Sprintf("frontend_url = %q", frontendURL)
		}
		return fmt.Sprintf(`
resource "skycloak_realm" "test" {
  cluster_id = %q
  name       = "tf-acc-realm-frontend"
  %s
}`, clusterID, attr)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccRealmDestroyed(clusterID),
		Steps: []resource.TestStep{
			{
				Config: config("https://login.example.com"),
				Check:  resource.TestCheckResourceAttr(res, "frontend_url", "https://login.example.com"),
			},
			{
				Config: config("https://sso.example.com:8443"),
				Check:  resource.TestCheckResourceAttr(res, "frontend_url", "https://sso.example.com:8443"),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: config(""),
				Check:  resource.TestCheckNoResourceAttr(res, "frontend_url"),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: config("https://login.example.com"),
				Check:  resource.TestCheckResourceAttr(res, "frontend_url", "https://login.example.com"),
			},
		},
	})
}
