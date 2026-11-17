package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

const privateLinkClusterID = "11111111-1111-1111-1111-111111111111"
const privateLinkSubscriptionID = "22222222-2222-2222-2222-222222222222"

func privateLinkSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	NewClusterPrivateLinkResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return resp.Schema
}

func privateLinkState(t *testing.T, m clusterPrivateLinkModel) tfsdk.State {
	t.Helper()
	s := privateLinkSchema(t)
	if m.AzureSubscriptionIDs.ElementType(context.Background()) == nil {
		m.AzureSubscriptionIDs = types.SetNull(types.StringType)
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
	if diags := state.Set(context.Background(), m); diags.HasError() {
		t.Fatal(diags)
	}
	return state
}

func privateLinkModel(t *testing.T, state tfsdk.State) clusterPrivateLinkModel {
	t.Helper()
	var m clusterPrivateLinkModel
	if diags := state.Get(context.Background(), &m); diags.HasError() {
		t.Fatal(diags)
	}
	return m
}

func TestClusterPrivateLinkResourceLifecycle(t *testing.T) {
	ctx := context.Background()
	enabled := false
	ids := []string{}
	calls := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method)
		if r.URL.Path != "/clusters/"+privateLinkClusterID+"/private-link" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Method == http.MethodPut {
			var body struct {
				Enabled bool      `json:"enabled"`
				IDs     *[]string `json:"azure_subscription_ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			enabled = body.Enabled
			if body.IDs != nil {
				ids = *body.IDs
			}
		} else if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		result := map[string]any{"enabled": enabled, "azure_subscription_ids": ids, "status": "disabled"}
		if enabled {
			result["alias"] = "example.alias"
			result["status"] = "waiting"
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	r := &clusterPrivateLinkResource{client: skycloak.New(srv.URL, "", "")}
	set, diags := types.SetValueFrom(ctx, types.StringType, []string{privateLinkSubscriptionID})
	if diags.HasError() {
		t.Fatal(diags)
	}
	m := clusterPrivateLinkModel{ClusterID: types.StringValue(privateLinkClusterID), Enabled: types.BoolValue(true), AzureSubscriptionIDs: set, ID: types.StringUnknown(), Alias: types.StringUnknown()}
	planned := privateLinkState(t, m)
	create := resource.CreateResponse{State: privateLinkState(t, clusterPrivateLinkModel{})}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan(planned)}, &create)
	if create.Diagnostics.HasError() {
		t.Fatal(create.Diagnostics)
	}
	m = privateLinkModel(t, create.State)
	if m.ID.ValueString() != privateLinkClusterID+"/private-link" || m.Alias.ValueString() != "example.alias" || !m.Enabled.ValueBool() || !m.AzureSubscriptionIDs.Equal(set) {
		t.Fatalf("unexpected create: %+v", m)
	}
	read := resource.ReadResponse{State: create.State}
	r.Read(ctx, resource.ReadRequest{State: create.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	if got := privateLinkModel(t, read.State); !got.Alias.Equal(m.Alias) {
		t.Fatal("refresh lost alias")
	}
	for _, importID := range []string{privateLinkClusterID, privateLinkClusterID + "/private-link"} {
		imported := resource.ImportStateResponse{State: privateLinkState(t, clusterPrivateLinkModel{})}
		r.ImportState(ctx, resource.ImportStateRequest{ID: importID}, &imported)
		if imported.Diagnostics.HasError() {
			t.Fatal(imported.Diagnostics)
		}
		refreshed := resource.ReadResponse{State: imported.State}
		r.Read(ctx, resource.ReadRequest{State: imported.State}, &refreshed)
		if refreshed.Diagnostics.HasError() {
			t.Fatal(refreshed.Diagnostics)
		}
		got := privateLinkModel(t, refreshed.State)
		if !got.ID.Equal(m.ID) || !got.ClusterID.Equal(m.ClusterID) || !got.Enabled.Equal(m.Enabled) || !got.Alias.Equal(m.Alias) || !got.AzureSubscriptionIDs.Equal(m.AzureSubscriptionIDs) {
			t.Fatalf("import = %+v", got)
		}
	}
	// Destroy disables the link without clearing the allowed subscriptions.
	deleted := resource.DeleteResponse{State: read.State}
	r.Delete(ctx, resource.DeleteRequest{State: read.State}, &deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
	if enabled || len(ids) != 1 || ids[0] != privateLinkSubscriptionID {
		t.Fatalf("destroy: enabled=%v ids=%v", enabled, ids)
	}
	m.Enabled = types.BoolValue(false)
	m.AzureSubscriptionIDs = types.SetValueMust(types.StringType, nil)
	m.Alias = types.StringUnknown()
	planned = privateLinkState(t, m)
	updated := resource.UpdateResponse{State: read.State}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan(planned), State: read.State}, &updated)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	m = privateLinkModel(t, updated.State)
	if m.Enabled.ValueBool() || !m.Alias.IsNull() || m.AzureSubscriptionIDs.IsNull() || len(m.AzureSubscriptionIDs.Elements()) != 0 || len(ids) != 0 {
		t.Fatalf("unexpected update: %+v", m)
	}
	if len(calls) != 6 {
		t.Fatalf("calls = %v", calls)
	}
}

func TestClusterPrivateLinkResourceErrors(t *testing.T) {
	for _, status := range []int{403, 404, 409, 422} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(status)
				_, _ = fmt.Fprintf(w, `{"title":"refused","status":%d}`, status)
			}))
			defer srv.Close()
			r := &clusterPrivateLinkResource{client: skycloak.New(srv.URL, "", "")}
			state := privateLinkState(t, clusterPrivateLinkModel{ID: types.StringValue(privateLinkClusterID + "/private-link"), ClusterID: types.StringValue(privateLinkClusterID), Enabled: types.BoolValue(true), AzureSubscriptionIDs: types.SetValueMust(types.StringType, nil), Alias: types.StringValue("example.alias")})
			read := resource.ReadResponse{State: state}
			r.Read(context.Background(), resource.ReadRequest{State: state}, &read)
			if status == 404 {
				if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
					t.Fatal("404 must remove state")
				}
			} else if !read.Diagnostics.HasError() || read.State.Raw.IsNull() {
				t.Fatal("failed refresh must preserve state")
			}
			deleted := resource.DeleteResponse{State: state}
			r.Delete(context.Background(), resource.DeleteRequest{State: state}, &deleted)
			if deleted.Diagnostics.HasError() != (status != 404) {
				t.Fatalf("delete diagnostics: %v", deleted.Diagnostics)
			}
			if status != 404 && deleted.State.Raw.IsNull() {
				t.Fatal("failed disable must preserve state")
			}
			plan := tfsdk.Plan(state)
			create := resource.CreateResponse{State: state}
			r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &create)
			if !create.Diagnostics.HasError() {
				t.Fatal("create must report API error")
			}
			update := resource.UpdateResponse{State: state}
			r.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: state}, &update)
			if !update.Diagnostics.HasError() {
				t.Fatal("update must report API error")
			}
			if calls != 4 {
				t.Fatalf("terminal failures retried: %d requests", calls)
			}
		})
	}
}

func TestClusterPrivateLinkSchema(t *testing.T) {
	s := privateLinkSchema(t)
	for _, name := range []string{"cluster_id", "enabled", "azure_subscription_ids"} {
		if !s.Attributes[name].IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}
	for _, name := range []string{"id", "alias"} {
		if !s.Attributes[name].IsComputed() {
			t.Errorf("%s must be computed", name)
		}
	}
	state := privateLinkState(t, clusterPrivateLinkModel{ClusterID: types.StringValue(privateLinkClusterID)})
	attr := s.Attributes["cluster_id"].(schema.StringAttribute)
	replace := false
	for _, modifier := range attr.PlanModifiers {
		resp := planmodifier.StringResponse{}
		modifier.PlanModifyString(context.Background(), planmodifier.StringRequest{Path: path.Root("cluster_id"), State: state, Plan: tfsdk.Plan(state), StateValue: types.StringValue(privateLinkClusterID), PlanValue: types.StringValue(privateLinkSubscriptionID)}, &resp)
		replace = replace || resp.RequiresReplace
	}
	if !replace {
		t.Fatal("cluster ID change must replace")
	}
}

func TestAccClusterPrivateLinkResource(t *testing.T) {
	// A dedicated Azure cluster with public access on is required for safe cleanup.
	clusterID := os.Getenv("SKYCLOAK_ACCEPTANCE_PRIVATE_LINK_CLUSTER_ID")
	subscriptionID := os.Getenv("SKYCLOAK_ACCEPTANCE_AZURE_SUBSCRIPTION_ID")
	if clusterID == "" || subscriptionID == "" {
		t.Skip("Azure private link acceptance cluster and subscription ID not set")
	}
	config := func(enabled bool) string {
		return fmt.Sprintf(`
resource "skycloak_cluster_private_link" "test" {
 cluster_id = %q
 enabled = %t
 azure_subscription_ids = [%q]
}`, clusterID, enabled, subscriptionID)
	}
	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(_ *terraform.State) error {
			link, err := testAccClient().GetClusterPrivateLink(context.Background(), clusterID)
			if err != nil {
				return err
			}
			if link.Enabled {
				return fmt.Errorf("private link is still enabled")
			}
			return nil
		},
		Steps: []testresource.TestStep{
			{Config: config(true), Check: testresource.ComposeAggregateTestCheckFunc(testresource.TestCheckResourceAttr("skycloak_cluster_private_link.test", "enabled", "true"), testresource.TestCheckResourceAttrSet("skycloak_cluster_private_link.test", "alias"), testresource.TestCheckResourceAttr("skycloak_cluster_private_link.test", "azure_subscription_ids.#", "1"))},
			{ResourceName: "skycloak_cluster_private_link.test", ImportState: true, ImportStateId: clusterID, ImportStateVerify: true},
			{Config: config(false), Check: testresource.TestCheckResourceAttr("skycloak_cluster_private_link.test", "enabled", "false")},
			{Config: config(true)},
		},
	})
}

func TestPrivateLinkComputedValues(t *testing.T) {
	ctx := context.Background()
	for _, alias := range []*string{nil, new(string)} {
		m := clusterPrivateLinkModel{ClusterID: types.StringValue(privateLinkClusterID), Alias: types.StringValue("old.alias")}
		diags := applyPrivateLinkToModel(ctx, &skycloak.ClusterPrivateLink{Enabled: true, Alias: alias}, &m)
		if diags.HasError() {
			t.Fatal(diags)
		}
		if !m.Alias.IsNull() {
			t.Fatal("unavailable alias must clear the old value")
		}
		if m.AzureSubscriptionIDs.IsNull() || len(m.AzureSubscriptionIDs.Elements()) != 0 {
			t.Fatal("absent subscriptions must read as an empty set")
		}
	}
	m := clusterPrivateLinkModel{ClusterID: types.StringValue(privateLinkClusterID)}
	diags := applyPrivateLinkToModel(ctx, &skycloak.ClusterPrivateLink{Enabled: true, AzureSubscriptionIDs: []string{privateLinkSubscriptionID, privateLinkClusterID}}, &m)
	if diags.HasError() {
		t.Fatal(diags)
	}
	reordered, diags := types.SetValueFrom(ctx, types.StringType, []string{privateLinkClusterID, privateLinkSubscriptionID})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !m.AzureSubscriptionIDs.Equal(reordered) {
		t.Fatal("subscription order must not cause a diff")
	}
}
