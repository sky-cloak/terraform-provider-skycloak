package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccWebhookSubscriptionResource exercises create → update → import →
// destroy. Workspace-level: needs no cluster, so it is cheap enough for the
// full suite on any run.
func TestAccWebhookSubscriptionResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "skycloak_webhook_subscription" "test" {
  name           = "tf-acc-webhook"
  url            = "https://hooks.invalid.example.com/skycloak-tf-acc"
  source         = "keycloak"
  event_types    = ["LOGIN_ERROR"]
  signing_secret = "whsec_tf_acc_test_secret_0123456789abcdef"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("skycloak_webhook_subscription.test", "id"),
					resource.TestCheckResourceAttr("skycloak_webhook_subscription.test", "enabled", "true"),
					resource.TestCheckResourceAttr("skycloak_webhook_subscription.test", "has_signing_secret", "true"),
				),
			},
			{
				Config: `
resource "skycloak_webhook_subscription" "test" {
  name           = "tf-acc-webhook-renamed"
  url            = "https://hooks.invalid.example.com/skycloak-tf-acc"
  source         = "keycloak"
  enabled        = false
  event_types    = ["LOGIN_ERROR", "LOGIN"]
  signing_secret = "whsec_tf_acc_test_secret_0123456789abcdef"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("skycloak_webhook_subscription.test", "name", "tf-acc-webhook-renamed"),
					resource.TestCheckResourceAttr("skycloak_webhook_subscription.test", "enabled", "false"),
					resource.TestCheckResourceAttr("skycloak_webhook_subscription.test", "event_types.#", "2"),
				),
			},
			{
				ResourceName:            "skycloak_webhook_subscription.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"signing_secret", "authorization_header"},
			},
		},
	})
}
