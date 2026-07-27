package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// TestAccRealmExportResource exercises create → read → import → destroy for a
// realm export against a pre-provisioned dev cluster. It creates a throwaway
// realm to export so the test does not depend on the cluster's existing realms.
func TestAccRealmExportResource(t *testing.T) {
	clusterID := testAccClusterID(t)
	const realm = "tf-acc-realm-export"
	config := fmt.Sprintf(`
resource "skycloak_realm" "src" {
  cluster_id = %q
  name       = %q
}

resource "skycloak_realm_export" "test" {
  cluster_id          = %q
  realm               = skycloak_realm.src.name
  encryption_password = "tf-acc-export-passphrase"
}`, clusterID, realm, clusterID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("skycloak_realm_export.test", "id"),
					resource.TestCheckResourceAttr("skycloak_realm_export.test", "realm", realm),
					resource.TestCheckResourceAttr("skycloak_realm_export.test", "scope", "full"),
					resource.TestCheckResourceAttr("skycloak_realm_export.test", "status", "completed"),
					resource.TestCheckResourceAttr("skycloak_realm_export.test", "progress", "100"),
					resource.TestCheckResourceAttrSet("skycloak_realm_export.test", "download_url"),
					resource.TestCheckResourceAttrSet("skycloak_realm_export.test", "sha256_checksum"),
					resource.TestCheckResourceAttrSet("skycloak_realm_export.test", "expires_at"),
				),
			},
			{
				ResourceName:      "skycloak_realm_export.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Write-only; the API never returns it.
				ImportStateVerifyIgnore: []string{"encryption_password"},
			},
		},
	})
}

// destroyImportedRealm deletes the realm the import created. Destroying
// skycloak_realm_import only drops the job from state and deliberately leaves
// the realm running, so without this cleanup the test passes once and then
// collides with its own leftovers on the next run.
func destroyImportedRealm(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "skycloak_realm_import" {
			continue
		}
		realm := rs.Primary.Attributes["realm"]
		clusterID := rs.Primary.Attributes["cluster_id"]
		if realm == "" || clusterID == "" {
			continue
		}
		if err := testAccClient().DeleteRealm(context.Background(), clusterID, realm); err != nil && !skycloak.IsNotFound(err) {
			return fmt.Errorf("cleaning up imported realm %q: %w", realm, err)
		}
	}
	return nil
}

// TestAccRealmImportResource imports a realm from a local artifact.
//
// It is gated on SKYCLOAK_ACCEPTANCE_REALM_ARTIFACT (path to an encrypted realm
// archive) and SKYCLOAK_ACCEPTANCE_REALM_ARTIFACT_PASSWORD, because a realm
// import cannot be round-tripped from an export inside the same cluster:
// preflight refuses a realm-name collision with the realm that was exported.
func TestAccRealmImportResource(t *testing.T) {
	clusterID := testAccClusterID(t)
	artifact := os.Getenv("SKYCLOAK_ACCEPTANCE_REALM_ARTIFACT")
	if artifact == "" {
		t.Skip("SKYCLOAK_ACCEPTANCE_REALM_ARTIFACT not set; skipping realm import acceptance test")
	}
	password := os.Getenv("SKYCLOAK_ACCEPTANCE_REALM_ARTIFACT_PASSWORD")

	config := fmt.Sprintf(`
resource "skycloak_realm_import" "test" {
  cluster_id  = %q
  source_file = %q
  password    = %q
}`, clusterID, artifact, password)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             destroyImportedRealm,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("skycloak_realm_import.test", "id"),
					resource.TestCheckResourceAttr("skycloak_realm_import.test", "source_kind", "upload"),
					resource.TestCheckResourceAttr("skycloak_realm_import.test", "status", "completed"),
					resource.TestCheckResourceAttrSet("skycloak_realm_import.test", "realm"),
					resource.TestCheckResourceAttrSet("skycloak_realm_import.test", "source_file_sha256"),
				),
			},
			{
				ResourceName:      "skycloak_realm_import.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Local-only or write-only; the API returns none of them.
				ImportStateVerifyIgnore: []string{"password", "source_file", "source_file_sha256"},
			},
		},
	})
}
