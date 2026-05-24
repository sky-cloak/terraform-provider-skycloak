package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
)

func TestProviderMetadata(t *testing.T) {
	p := New("test")()

	var resp provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &resp)

	if resp.TypeName != "skycloak" {
		t.Fatalf("TypeName = %q, want skycloak", resp.TypeName)
	}
	if len(p.Resources(context.Background())) == 0 {
		t.Fatal("expected at least one resource")
	}
	if len(p.DataSources(context.Background())) == 0 {
		t.Fatal("expected at least one data source")
	}
}

func TestProviderSchema(t *testing.T) {
	p := New("test")()

	var resp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, attr := range []string{"endpoint", "api_key", "api_version"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("missing provider attribute %q", attr)
		}
	}
}
