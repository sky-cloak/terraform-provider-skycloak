package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	acc "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// testAccOpenFGAInstanceDestroyed fails when any OpenFGA instance created by
// the test is still listed on the cluster after the run.
func testAccOpenFGAInstanceDestroyed(clusterID string) acc.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "skycloak_openfga_instance" {
				continue
			}
			_, err := testAccClient().GetOpenFGAInstance(context.Background(), clusterID, rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("OpenFGA instance %s still exists", rs.Primary.ID)
			}
			if !skycloak.IsNotFound(err) {
				return fmt.Errorf("checking OpenFGA instance %s: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

// TestAccOpenFGAInstanceResource exercises create → read → import → replace on
// realm change → destroy against a pre-provisioned dev cluster
// (SKYCLOAK_ACCEPTANCE_CLUSTER_ID) in a workspace with OpenFGA access. It binds
// the instance to throwaway realms so the test does not depend on the cluster's
// existing realms, and checks the data source against the same instance.
func TestAccOpenFGAInstanceResource(t *testing.T) {
	clusterID := testAccClusterID(t)
	config := func(realm string) string {
		return fmt.Sprintf(`
resource "skycloak_realm" "fga" {
  cluster_id = %[1]q
  name       = %[2]q
}

resource "skycloak_openfga_instance" "test" {
  cluster_id = %[1]q
  realm      = skycloak_realm.fga.name
}

data "skycloak_openfga_instance" "by_id" {
  cluster_id = %[1]q
  id         = skycloak_openfga_instance.test.id
}

data "skycloak_openfga_instance" "by_realm" {
  cluster_id = %[1]q
  realm      = skycloak_openfga_instance.test.realm
}`, clusterID, realm)
	}

	const res = "skycloak_openfga_instance.test"
	acc.Test(t, acc.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccOpenFGAInstanceDestroyed(clusterID),
		Steps: []acc.TestStep{
			{
				Config: config("tf-acc-fga-a"),
				Check: acc.ComposeAggregateTestCheckFunc(
					acc.TestCheckResourceAttr(res, "realm", "tf-acc-fga-a"),
					acc.TestCheckResourceAttr(res, "status", "ready"),
					acc.TestCheckResourceAttrSet(res, "id"),
					acc.TestCheckResourceAttrSet(res, "api_url"),
					acc.TestCheckResourceAttrSet(res, "issuer"),
					acc.TestCheckResourceAttrSet(res, "audience"),
					acc.TestCheckResourceAttrPair("data.skycloak_openfga_instance.by_id", "api_url", res, "api_url"),
					acc.TestCheckResourceAttrPair("data.skycloak_openfga_instance.by_id", "realm", res, "realm"),
					acc.TestCheckResourceAttrPair("data.skycloak_openfga_instance.by_realm", "id", res, "id"),
					acc.TestCheckResourceAttrPair("data.skycloak_openfga_instance.by_realm", "audience", res, "audience"),
				),
			},
			{
				ResourceName: res,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return clusterID + "/" + s.RootModule().Resources[res].Primary.ID, nil
				},
				ImportStateVerify: true,
			},
			{
				// A different realm is a different instance: the old one is
				// removed and a new one provisioned.
				Config: config("tf-acc-fga-b"),
				Check: acc.ComposeAggregateTestCheckFunc(
					acc.TestCheckResourceAttr(res, "realm", "tf-acc-fga-b"),
					acc.TestCheckResourceAttr(res, "status", "ready"),
				),
			},
		},
	})
}

// TestOpenFGAInstanceSchema checks, without a server, that the realm and the
// cluster force replacement and that the server-chosen values are computed.
func TestOpenFGAInstanceSchema(t *testing.T) {
	r := NewOpenFGAInstanceResource()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"cluster_id", "realm"} {
		a, ok := resp.Schema.Attributes[name]
		if !ok || !a.IsRequired() {
			t.Fatalf("%s must be a required attribute", name)
		}
	}
	for _, name := range []string{"id", "status", "status_message", "api_url", "issuer", "audience", "created_at", "updated_at"} {
		a, ok := resp.Schema.Attributes[name]
		if !ok || !a.IsComputed() || a.IsRequired() {
			t.Fatalf("%s must be computed only", name)
		}
	}
	if d := resp.Schema.ValidateImplementation(context.Background()); d.HasError() {
		t.Fatalf("schema implementation: %v", d)
	}
}

func TestOpenFGAInstanceImportState(t *testing.T) {
	r := NewOpenFGAInstanceResource().(resource.ResourceWithImportState)
	var sresp resource.SchemaResponse
	NewOpenFGAInstanceResource().Schema(context.Background(), resource.SchemaRequest{}, &sresp)

	const clusterID = "11111111-1111-1111-1111-111111111111"
	const instanceID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"

	for _, tc := range []struct {
		name, id string
		wantErr  bool
	}{
		{"valid", clusterID + "/" + instanceID, false},
		{"no separator", instanceID, true},
		{"empty cluster", "/" + instanceID, true},
		{"empty instance", clusterID + "/", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := tfsdk.State{Schema: sresp.Schema, Raw: tftypes.NewValue(sresp.Schema.Type().TerraformType(context.Background()), nil)}
			resp := resource.ImportStateResponse{State: state}
			r.ImportState(context.Background(), resource.ImportStateRequest{ID: tc.id}, &resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("diagnostics = %v, wantErr %v", resp.Diagnostics, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			var m openFGAInstanceModel
			if d := resp.State.Get(context.Background(), &m); d.HasError() {
				t.Fatalf("reading state: %v", d)
			}
			if m.ClusterID.ValueString() != clusterID || m.ID.ValueString() != instanceID {
				t.Fatalf("imported %q / %q", m.ClusterID.ValueString(), m.ID.ValueString())
			}
		})
	}
}
