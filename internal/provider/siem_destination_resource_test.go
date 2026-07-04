package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// TestAccSIEMDestinationResource exercises create → update → import → destroy.
// SIEM forwarding is plan-gated; on a workspace without it the API returns
// 403 and the test skips rather than fails.
func TestAccSIEMDestinationResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t); testAccSkipWithoutSIEM(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "skycloak_siem_destination" "test" {
  name = "tf-acc-syslog"
  type = "syslog"
  source = {
    type = "security_logs"
  }
  syslog = {
    host     = "siem.invalid.example.com"
    port     = 6514
    protocol = "tls"
    format   = "rfc5424"
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("skycloak_siem_destination.test", "id"),
					resource.TestCheckResourceAttr("skycloak_siem_destination.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("skycloak_siem_destination.test", "health_status"),
				),
			},
			{
				Config: `
resource "skycloak_siem_destination" "test" {
  name    = "tf-acc-syslog-renamed"
  type    = "syslog"
  enabled = false
  source = {
    type = "security_logs"
  }
  syslog = {
    host     = "siem.invalid.example.com"
    port     = 6514
    protocol = "tls"
    format   = "json"
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skycloak_siem_destination.test", "name", "tf-acc-syslog-renamed"),
					resource.TestCheckResourceAttr("skycloak_siem_destination.test", "enabled", "false"),
					resource.TestCheckResourceAttr("skycloak_siem_destination.test", "syslog.format", "json"),
				),
			},
			{
				ResourceName:      "skycloak_siem_destination.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccSkipWithoutSIEM probes the SIEM list endpoint and skips the test when
// the workspace's plan does not include SIEM forwarding (403).
func testAccSkipWithoutSIEM(t *testing.T) {
	t.Helper()
	client := testAccClient()
	if _, err := client.ListSIEMDestinations(t.Context()); err != nil {
		if apiErr, ok := skycloak.AsAPIError(err); ok && apiErr.StatusCode == 403 {
			t.Skip("workspace plan does not include SIEM forwarding; skipping")
		}
		t.Fatalf("probing SIEM availability: %v", err)
	}
}
