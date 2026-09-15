package provider

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// writeThemeZip writes a minimal but valid Keycloak login theme archive at
// path. marker varies the stylesheet so two archives differ in bytes.
func writeThemeZip(t *testing.T, path, themeName, marker string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		themeName + "/login/theme.properties":         "parent=keycloak\nstyles=css/styles.css\n",
		themeName + "/login/resources/css/styles.css": "/* " + marker + " */\n.login-pf body { background: #fff; }\n",
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write zip: %v", err)
	}
}

// checkThemeAssignmentUnchanged asserts the realm still points at the theme
// after its content was replaced: an in-place content update must leave the
// assignment alone, which a destroy/create could not.
func checkThemeAssignmentUnchanged(clusterID, realm string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["skycloak_custom_theme.test"]
		if !ok {
			return fmt.Errorf("skycloak_custom_theme.test not in state")
		}
		assignment, err := testAccClient().GetThemeAssignment(context.Background(), clusterID, realm)
		if err != nil {
			return fmt.Errorf("reading theme assignment for realm %s: %w", realm, err)
		}
		if assignment.Login != rs.Primary.ID {
			return fmt.Errorf("realm login theme = %q, want the updated theme %q", assignment.Login, rs.Primary.ID)
		}
		return nil
	}
}

// TestAccCustomThemeResource exercises create → in-place content update →
// import → destroy against a pre-provisioned dev cluster. Step two rewrites the
// archive on disk and leaves the configuration untouched, which is the case
// that used to force a destroy/create: the plan check fails if Terraform still
// plans a replacement, and the realm assignment check fails if the theme was
// swapped out underneath the realm.
func TestAccCustomThemeResource(t *testing.T) {
	clusterID := testAccClusterID(t)
	const (
		realm     = "tf-acc-theme"
		themeName = "tfacctheme"
	)
	archive := filepath.Join(t.TempDir(), "theme.zip")
	writeThemeZip(t, archive, themeName, "v1")

	config := fmt.Sprintf(`
resource "skycloak_realm" "test" {
  cluster_id = %q
  name       = %q
}

resource "skycloak_custom_theme" "test" {
  cluster_id = %q
  source     = %q
  name       = %q
  version    = "1.0.0"
}

resource "skycloak_theme_assignment" "test" {
  cluster_id = %q
  realm_name = skycloak_realm.test.name
  login      = skycloak_custom_theme.test.id
}`, clusterID, realm, clusterID, archive, themeName, clusterID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("skycloak_custom_theme.test", "id"),
					resource.TestCheckResourceAttr("skycloak_custom_theme.test", "name", themeName),
					resource.TestCheckResourceAttr("skycloak_custom_theme.test", "status", "deployed"),
					resource.TestCheckResourceAttrSet("skycloak_custom_theme.test", "content_sha256"),
				),
			},
			{
				// Same configuration, new archive bytes.
				PreConfig: func() { writeThemeZip(t, archive, themeName, "v2") },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("skycloak_custom_theme.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skycloak_custom_theme.test", "name", themeName),
					resource.TestCheckResourceAttr("skycloak_custom_theme.test", "status", "deployed"),
					checkThemeAssignmentUnchanged(clusterID, realm),
				),
			},
			{
				ResourceName: "skycloak_custom_theme.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["skycloak_custom_theme.test"]
					return clusterID + "/" + rs.Primary.ID, nil
				},
				ImportStateVerify: true,
				// Local-file attributes have no API counterpart.
				ImportStateVerifyIgnore: []string{"source", "content_sha256"},
			},
		},
	})
}
