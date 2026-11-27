package provider

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// configureProvider runs the provider's Configure with the given provider
// arguments and returns what it hands to resources.
func configureProvider(t *testing.T, args map[string]string) (*providerClients, provider.ConfigureResponse) {
	t.Helper()
	ctx := context.Background()
	p := New("test")()

	var schemaResp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)

	objType := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	vals := map[string]tftypes.Value{}
	for name := range objType.AttributeTypes {
		if v, ok := args[name]; ok {
			vals[name] = tftypes.NewValue(tftypes.String, v)
		} else {
			vals[name] = tftypes.NewValue(tftypes.String, nil)
		}
	}
	req := provider.ConfigureRequest{Config: tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(objType, vals),
	}}
	var resp provider.ConfigureResponse
	p.Configure(ctx, req, &resp)

	clients, _ := resp.ResourceData.(*providerClients)
	return clients, resp
}

func clearProductEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SKYCLOAK_API_KEY", "SKYCLOAK_PLATFORM_API_KEY", "SKYCLOAK_PLATFORM_ENDPOINT", "SKYCLOAK_ENDPOINT", "SKYCLOAK_API_VERSION"} {
		t.Setenv(k, "")
	}
}

func TestConfigureWithOnlyAPIKey(t *testing.T) {
	clearProductEnv(t)
	clients, resp := configureProvider(t, map[string]string{"api_key": "sk_sc_test_aaa_bbb"})
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if clients.keycloak == nil {
		t.Error("expected a managed Keycloak client")
	}
	if clients.platform != nil {
		t.Error("expected no Platform client without a Platform key")
	}
}

func TestConfigureWithNoKeysIsNotAnError(t *testing.T) {
	clearProductEnv(t)
	clients, resp := configureProvider(t, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if clients.keycloak != nil || clients.platform != nil {
		t.Error("expected no clients without keys")
	}
}

func TestConfigureKeysFromEnvironment(t *testing.T) {
	clearProductEnv(t)
	t.Setenv("SKYCLOAK_API_KEY", "sk_sc_test_aaa_bbb")
	t.Setenv("SKYCLOAK_PLATFORM_API_KEY", "platform-key")
	t.Setenv("SKYCLOAK_PLATFORM_ENDPOINT", "https://platform.example.test")
	clients, resp := configureProvider(t, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if clients.keycloak == nil || clients.platform == nil {
		t.Fatalf("expected both clients, got %+v", clients)
	}
}

func TestConfigureArgumentsBeatEnvironment(t *testing.T) {
	clearProductEnv(t)
	t.Setenv("SKYCLOAK_PLATFORM_API_KEY", "")
	clients, _ := configureProvider(t, map[string]string{"platform_api_key": "platform-key", "platform_endpoint": "https://platform.example.test"})
	if clients.platform == nil {
		t.Fatal("expected a Platform client from the provider arguments")
	}
}

// stubResource records the provider data it is configured with.
type stubResource struct{ got any }

func (s *stubResource) Metadata(_ context.Context, req fwresource.MetadataRequest, resp *fwresource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stub"
}
func (s *stubResource) Schema(context.Context, fwresource.SchemaRequest, *fwresource.SchemaResponse) {
}
func (s *stubResource) Create(context.Context, fwresource.CreateRequest, *fwresource.CreateResponse) {
}
func (s *stubResource) Read(context.Context, fwresource.ReadRequest, *fwresource.ReadResponse) {}
func (s *stubResource) Update(context.Context, fwresource.UpdateRequest, *fwresource.UpdateResponse) {
}
func (s *stubResource) Delete(context.Context, fwresource.DeleteRequest, *fwresource.DeleteResponse) {
}
func (s *stubResource) Configure(_ context.Context, req fwresource.ConfigureRequest, _ *fwresource.ConfigureResponse) {
	s.got = req.ProviderData
}

func TestGatedResourceNamesAttributeAndEnvVar(t *testing.T) {
	for _, tc := range []struct {
		product        product
		attribute, env string
	}{
		{keycloakProduct, "api_key", "SKYCLOAK_API_KEY"},
		{platformProduct, "platform_api_key", "SKYCLOAK_PLATFORM_API_KEY"},
	} {
		inner := &stubResource{}
		r := gateResource(tc.product, func() fwresource.Resource { return inner })()

		var resp fwresource.ConfigureResponse
		r.(fwresource.ResourceWithConfigure).Configure(context.Background(),
			fwresource.ConfigureRequest{ProviderData: &providerClients{}}, &resp)

		if !resp.Diagnostics.HasError() {
			t.Fatalf("%s: expected an error when the key is missing", tc.attribute)
		}
		msg := resp.Diagnostics[0].Summary() + " " + resp.Diagnostics[0].Detail()
		for _, want := range []string{tc.attribute, tc.env, "skycloak_stub"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: error %q does not mention %q", tc.attribute, msg, want)
			}
		}
		if inner.got != nil {
			t.Errorf("%s: the resource was configured without a key", tc.attribute)
		}
	}
}

func TestGatedResourceGetsItsProductClient(t *testing.T) {
	keycloak := skycloak.New("", "sk_sc_test_aaa_bbb", "")
	platform := skycloak.NewPlatform("", "platform-key")
	clients := &providerClients{keycloak: keycloak, platform: platform}

	for _, tc := range []struct {
		product product
		want    any
	}{{keycloakProduct, keycloak}, {platformProduct, platform}} {
		inner := &stubResource{}
		r := gateResource(tc.product, func() fwresource.Resource { return inner })()
		var resp fwresource.ConfigureResponse
		r.(fwresource.ResourceWithConfigure).Configure(context.Background(),
			fwresource.ConfigureRequest{ProviderData: clients}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("diagnostics: %v", resp.Diagnostics)
		}
		if inner.got != tc.want {
			t.Errorf("%s: got %T, want the product's own client", tc.product.name, inner.got)
		}
	}
}

func TestGatedResourceSkipsConfigureBeforeProviderIsConfigured(t *testing.T) {
	inner := &stubResource{}
	r := gateResource(keycloakProduct, func() fwresource.Resource { return inner })()
	var resp fwresource.ConfigureResponse
	r.(fwresource.ResourceWithConfigure).Configure(context.Background(), fwresource.ConfigureRequest{}, &resp)
	if resp.Diagnostics.HasError() || inner.got != nil {
		t.Fatalf("expected a no-op, got %v and %v", resp.Diagnostics, inner.got)
	}
}

func TestEveryRegisteredObjectIsGated(t *testing.T) {
	ctx := context.Background()
	p := New("test")()
	for _, f := range p.Resources(ctx) {
		if _, ok := f().(*gatedResource); !ok {
			t.Errorf("resource %T is not gated by its product key", f())
		}
	}
	for _, f := range p.DataSources(ctx) {
		if _, ok := f().(*gatedDataSource); !ok {
			t.Errorf("data source %T is not gated by its product key", f())
		}
	}
	for _, f := range p.(provider.ProviderWithActions).Actions(ctx) {
		if _, ok := f().(*gatedAction); !ok {
			t.Errorf("action %T is not gated by its product key", f())
		}
	}
}

// A resource without the managed Keycloak key fails at plan, naming the
// provider argument and the environment variable.
func TestResourceWithoutKeyFailsAtPlan(t *testing.T) {
	clearProductEnv(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "skycloak" {
  platform_api_key = "platform-key"
}

resource "skycloak_cluster" "test" {
  name     = "tf-keys"
  type     = "keycloak"
  size     = "small"
  version  = "26.1"
  location = "eu"
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Missing managed Keycloak API key.*api_key.*SKYCLOAK_API_KEY`),
			},
		},
	})
}
