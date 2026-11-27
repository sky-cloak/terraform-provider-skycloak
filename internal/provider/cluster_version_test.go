package provider

import (
	"context"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestIsVersionDowngrade(t *testing.T) {
	for _, tc := range []struct {
		configured, current string
		want                bool
	}{
		{"26.2.4", "26.8.2", true},
		{"25.9.9", "26.0.0", true},
		{"26.8.1", "26.8.2", true},
		{"26.8.2", "26.8.2", false},
		{"26.9.0", "26.8.2", false},
		{"27", "26.8.2", false},
		{"26.1", "26.1.3", false},
		{"26.1.3", "26.1", false},
		{"26.0", "26.1.3", true},
		{"latest", "26.8.2", false},
		{"26.8.2", "unknown", false},
		{"26..2", "26.8.2", false},
		{"", "26.8.2", false},
	} {
		if got := isVersionDowngrade(tc.configured, tc.current); got != tc.want {
			t.Errorf("isVersionDowngrade(%q, %q) = %v, want %v", tc.configured, tc.current, got, tc.want)
		}
	}
}

func TestClusterModifyPlanVersion(t *testing.T) {
	ctx := context.Background()
	r := &clusterResource{}
	var schemaResp frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)
	schema := schemaResp.Schema
	raw := func(version, size string, autoUpgrade bool) tftypes.Value {
		t.Helper()
		state := tfsdk.State{Schema: schema}
		diags := state.Set(ctx, clusterModel{
			ID:                 types.StringValue("11111111-1111-1111-1111-111111111111"),
			Name:               types.StringValue("main"),
			Type:               types.StringValue("keycloak"),
			Size:               types.StringValue(size),
			Version:            types.StringValue(version),
			Location:           types.StringValue("eu"),
			Site:               types.StringValue("eu-central"),
			Status:             types.StringValue("available"),
			URL:                types.StringValue("https://example.invalid"),
			AutoUpgradeEnabled: types.BoolValue(autoUpgrade),
		})
		if diags.HasError() {
			t.Fatal(diags)
		}
		return state.Raw
	}
	absent := tftypes.NewValue(schema.Type().TerraformType(ctx), nil)
	for _, tc := range []struct {
		name        string
		state, plan tftypes.Value
		wantError   bool
	}{
		{"downgrade", raw("26.8.2", "small", false), raw("26.2.4", "small", false), true},
		{"downgrade after auto-upgrade moved the cluster", raw("26.8.2", "small", true), raw("26.8.0", "small", true), true},
		{"equal", raw("26.8.2", "small", false), raw("26.8.2", "small", false), false},
		{"upgrade", raw("26.8.2", "small", false), raw("26.9.0", "small", false), false},
		{"prefix of current", raw("26.1.3", "small", false), raw("26.1", "small", false), false},
		{"unparseable configured", raw("26.8.2", "small", false), raw("latest", "small", false), false},
		{"unparseable current", raw("custom", "small", false), raw("26.2.4", "small", false), false},
		{"size change only", raw("26.8.2", "small", false), raw("26.8.2", "large", false), false},
		{"auto upgrade change only", raw("26.8.2", "small", false), raw("26.8.2", "small", true), false},
		{"create", absent, raw("26.2.4", "small", false), false},
		{"destroy", raw("26.8.2", "small", false), absent, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := frameworkresource.ModifyPlanRequest{
				Plan:  tfsdk.Plan{Schema: schema, Raw: tc.plan},
				State: tfsdk.State{Schema: schema, Raw: tc.state},
			}
			var resp frameworkresource.ModifyPlanResponse
			r.ModifyPlan(ctx, req, &resp)
			if resp.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("error = %v, want error %v", resp.Diagnostics, tc.wantError)
			}
			if tc.wantError {
				d := resp.Diagnostics.Errors()[0]
				if !strings.Contains(d.Detail(), `"26.`) || !strings.Contains(d.Detail(), "or higher") {
					t.Fatalf("unexpected detail: %s", d.Detail())
				}
			}
		})
	}
}

func TestClusterVersionErrorNamesBothVersions(t *testing.T) {
	ctx := context.Background()
	r := &clusterResource{}
	var schemaResp frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)
	build := func(version string) tftypes.Value {
		state := tfsdk.State{Schema: schemaResp.Schema}
		state.Set(ctx, clusterModel{
			ID: types.StringValue("x"), Name: types.StringValue("n"), Type: types.StringValue("keycloak"),
			Size: types.StringValue("small"), Version: types.StringValue(version), Location: types.StringValue("eu"),
			Site: types.StringValue("s"), Status: types.StringValue("available"), URL: types.StringValue("u"),
			AutoUpgradeEnabled: types.BoolValue(false),
		})
		return state.Raw
	}
	req := frameworkresource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: build("26.2.4")},
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: build("26.8.2")},
	}
	var resp frameworkresource.ModifyPlanResponse
	r.ModifyPlan(ctx, req, &resp)
	got := resp.Diagnostics.Errors()[0].Detail()
	want := `The configured version "26.2.4" is lower than the cluster's current version "26.8.2", and a Keycloak version cannot be downgraded. Set version to "26.8.2" or higher.`
	if got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}
}
