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

const publicAccessClusterID = "11111111-1111-1111-1111-111111111111"

func publicAccessSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	NewClusterPublicAccessResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return resp.Schema
}

func publicAccessState(t *testing.T, m clusterPublicAccessModel) tfsdk.State {
	t.Helper()
	s := publicAccessSchema(t)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
	if diags := state.Set(context.Background(), m); diags.HasError() {
		t.Fatal(diags)
	}
	return state
}

func publicAccessModel(t *testing.T, state tfsdk.State) clusterPublicAccessModel {
	t.Helper()
	var m clusterPublicAccessModel
	if diags := state.Get(context.Background(), &m); diags.HasError() {
		t.Fatal(diags)
	}
	return m
}

func publicAccessServer(t *testing.T, publicAccess *bool, calls *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.Method)
		if r.URL.Path != "/clusters/"+publicAccessClusterID {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			var body struct {
				PublicAccess *bool `json:"public_access"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PublicAccess == nil {
				t.Errorf("body must carry public_access: %v", err)
			} else {
				*publicAccess = *body.PublicAccess
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"name":"c","type":"DEDICATED","size":"SMALL","version":"26.0.0","location":"us","status":"READY","url":"https://c.example","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","http_relative_path":"","management_mode":"FULLY_MANAGED","public_access":%t}`, publicAccessClusterID, *publicAccess)
	}))
}

func TestClusterPublicAccessResourceLifecycle(t *testing.T) {
	ctx := context.Background()
	publicAccess := true
	calls := []string{}
	srv := publicAccessServer(t, &publicAccess, &calls)
	defer srv.Close()
	r := &clusterPublicAccessResource{client: skycloak.New(srv.URL, "", "")}
	planned := publicAccessState(t, clusterPublicAccessModel{ClusterID: types.StringValue(publicAccessClusterID), PublicAccess: types.BoolValue(false), ID: types.StringUnknown()})
	create := resource.CreateResponse{State: publicAccessState(t, clusterPublicAccessModel{})}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan(planned)}, &create)
	if create.Diagnostics.HasError() {
		t.Fatal(create.Diagnostics)
	}
	m := publicAccessModel(t, create.State)
	if m.ID.ValueString() != publicAccessClusterID+"/public-access" || m.PublicAccess.ValueBool() || publicAccess {
		t.Fatalf("unexpected create: %+v", m)
	}
	read := resource.ReadResponse{State: create.State}
	r.Read(ctx, resource.ReadRequest{State: create.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	if got := publicAccessModel(t, read.State); got.PublicAccess.ValueBool() {
		t.Fatal("refresh must read public access off")
	}
	for _, importID := range []string{publicAccessClusterID, publicAccessClusterID + "/public-access"} {
		imported := resource.ImportStateResponse{State: publicAccessState(t, clusterPublicAccessModel{})}
		r.ImportState(ctx, resource.ImportStateRequest{ID: importID}, &imported)
		if imported.Diagnostics.HasError() {
			t.Fatal(imported.Diagnostics)
		}
		refreshed := resource.ReadResponse{State: imported.State}
		r.Read(ctx, resource.ReadRequest{State: imported.State}, &refreshed)
		if refreshed.Diagnostics.HasError() {
			t.Fatal(refreshed.Diagnostics)
		}
		got := publicAccessModel(t, refreshed.State)
		if !got.ID.Equal(m.ID) || !got.ClusterID.Equal(m.ClusterID) || !got.PublicAccess.Equal(m.PublicAccess) {
			t.Fatalf("import = %+v", got)
		}
	}
	// Destroy turns public access back on.
	deleted := resource.DeleteResponse{State: read.State}
	r.Delete(ctx, resource.DeleteRequest{State: read.State}, &deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
	if !publicAccess {
		t.Fatal("destroy must restore public access")
	}
	m.PublicAccess = types.BoolValue(false)
	updated := resource.UpdateResponse{State: read.State}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan(publicAccessState(t, m)), State: read.State}, &updated)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	if publicAccessModel(t, updated.State).PublicAccess.ValueBool() || publicAccess {
		t.Fatal("update must turn public access off")
	}
	if len(calls) != 6 {
		t.Fatalf("calls = %v", calls)
	}
}

func TestClusterPublicAccessResourceErrors(t *testing.T) {
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
			r := &clusterPublicAccessResource{client: skycloak.New(srv.URL, "", "")}
			state := publicAccessState(t, clusterPublicAccessModel{ID: types.StringValue(publicAccessClusterID + "/public-access"), ClusterID: types.StringValue(publicAccessClusterID), PublicAccess: types.BoolValue(false)})
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
			plan := tfsdk.Plan(state)
			create := resource.CreateResponse{State: state}
			r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &create)
			if !create.Diagnostics.HasError() {
				t.Fatal("create must report the refusal")
			}
			update := resource.UpdateResponse{State: state}
			r.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: state}, &update)
			if !update.Diagnostics.HasError() {
				t.Fatal("update must report the refusal")
			}
			if calls != 4 {
				t.Fatalf("terminal failures retried: %d requests", calls)
			}
		})
	}
}

func TestClusterPublicAccessSchema(t *testing.T) {
	s := publicAccessSchema(t)
	for _, name := range []string{"cluster_id", "public_access"} {
		if !s.Attributes[name].IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}
	if !s.Attributes["id"].IsComputed() {
		t.Error("id must be computed")
	}
	state := publicAccessState(t, clusterPublicAccessModel{ClusterID: types.StringValue(publicAccessClusterID)})
	attr := s.Attributes["cluster_id"].(schema.StringAttribute)
	replace := false
	for _, modifier := range attr.PlanModifiers {
		resp := planmodifier.StringResponse{}
		modifier.PlanModifyString(context.Background(), planmodifier.StringRequest{Path: path.Root("cluster_id"), State: state, Plan: tfsdk.Plan(state), StateValue: types.StringValue(publicAccessClusterID), PlanValue: types.StringValue("22222222-2222-2222-2222-222222222222")}, &resp)
		replace = replace || resp.RequiresReplace
	}
	if !replace {
		t.Fatal("cluster ID change must replace")
	}
}

func TestAccClusterPublicAccessResource(t *testing.T) {
	// A dedicated Azure cluster with private link enabled and ready is required to turn public access off.
	clusterID := os.Getenv("SKYCLOAK_ACCEPTANCE_PRIVATE_LINK_CLUSTER_ID")
	if clusterID == "" {
		t.Skip("Azure private link acceptance cluster not set")
	}
	config := func(publicAccess bool) string {
		return fmt.Sprintf(`
resource "skycloak_cluster_public_access" "test" {
 cluster_id = %q
 public_access = %t
}`, clusterID, publicAccess)
	}
	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(_ *terraform.State) error {
			on, err := testAccClient().GetClusterPublicAccess(context.Background(), clusterID)
			if err != nil {
				return err
			}
			if !on {
				return fmt.Errorf("public access is still off")
			}
			return nil
		},
		Steps: []testresource.TestStep{
			{Config: config(false), Check: testresource.TestCheckResourceAttr("skycloak_cluster_public_access.test", "public_access", "false")},
			{ResourceName: "skycloak_cluster_public_access.test", ImportState: true, ImportStateId: clusterID, ImportStateVerify: true},
			{Config: config(true), Check: testresource.TestCheckResourceAttr("skycloak_cluster_public_access.test", "public_access", "true")},
		},
	})
}
