package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

const (
	testFeaturesClusterID = "11111111-1111-1111-1111-111111111111"
	clusterFeaturesAddr   = "skycloak_cluster_features.test"
)

func TestClusterFeaturesModifyPlan(t *testing.T) {
	ctx := context.Background()
	r := &clusterFeaturesResource{}
	var schemaResp frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)
	schema := schemaResp.Schema
	rawModel := func(flags []string, applyNow bool) tftypes.Value {
		t.Helper()
		featureSet, diags := types.SetValueFrom(ctx, types.StringType, flags)
		if diags.HasError() {
			t.Fatal(diags)
		}
		state := tfsdk.State{Schema: schema}
		diags = state.Set(ctx, clusterFeaturesResourceModel{
			ID:        types.StringValue(testFeaturesClusterID + "/features"),
			ClusterID: types.StringValue(testFeaturesClusterID),
			Features:  featureSet,
			ApplyNow:  types.BoolValue(applyNow),
		})
		if diags.HasError() {
			t.Fatal(diags)
		}
		return state.Raw
	}
	absent := tftypes.NewValue(schema.Type().TerraformType(ctx), nil)
	original := rawModel([]string{"cimd", "resource-indicators"}, false)
	for _, tc := range []struct {
		name        string
		state, plan tftypes.Value
		warning     bool
	}{
		{"create", absent, original, true},
		{"create with no flags", absent, rawModel([]string{}, false), true},
		{"remove a flag", original, rawModel([]string{"cimd"}, false), true},
		{"reorder flags", original, rawModel([]string{"resource-indicators", "cimd"}, false), false},
		{"apply pending changes now", original, rawModel([]string{"cimd", "resource-indicators"}, true), true},
		{"destroy", original, absent, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := frameworkresource.ModifyPlanRequest{
				Plan:  tfsdk.Plan{Schema: schema, Raw: tc.plan},
				State: tfsdk.State{Schema: schema, Raw: tc.state},
			}
			var resp frameworkresource.ModifyPlanResponse
			r.ModifyPlan(ctx, req, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			warnings := resp.Diagnostics.Warnings()
			if (len(warnings) > 0) != tc.warning {
				t.Fatalf("warnings = %v, want warning %v", warnings, tc.warning)
			}
			if tc.warning && !strings.Contains(warnings[0].Summary(), "can restart Keycloak") {
				t.Fatalf("unexpected warning: %s", warnings[0].Summary())
			}
		})
	}
}

func TestClusterFeaturesReplace(t *testing.T) {
	for _, tc := range []struct {
		name          string
		applyNow      bool
		response      string
		clusterStatus string
		wantError     string
	}{
		{
			name:      "scheduled despite apply now",
			applyNow:  true,
			response:  `{"features":["cimd"],"change":{"state":"pending"}}`,
			wantError: "the API scheduled the restart for the maintenance window instead of starting it immediately",
		},
		{
			name:      "different flags returned",
			response:  `{"features":["resource-indicators"]}`,
			wantError: "the API returned a different set of enabled flags than requested",
		},
		{
			name:          "applying without apply now",
			response:      `{"features":["cimd"],"change":{"state":"applying"}}`,
			clusterStatus: "available",
		},
		{
			name:          "restart fails after flags are saved",
			applyNow:      true,
			response:      `{"features":["cimd"],"change":{"state":"applying"}}`,
			clusterStatus: "degraded",
			wantError:     "the API saved the requested feature flags. Waiting for Keycloak restart failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var readinessChecks atomic.Int32
			clusterPath := "/clusters/" + testFeaturesClusterID
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.Method == http.MethodPut && req.URL.Path == clusterPath+"/features":
					var input struct {
						Features []string `json:"features"`
						ApplyNow *bool    `json:"apply_now"`
					}
					if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
						t.Error(err)
					} else if !slices.Equal(input.Features, []string{"cimd"}) || input.ApplyNow == nil || *input.ApplyNow != tc.applyNow {
						t.Error("unexpected feature replacement request")
					}
					_, _ = io.WriteString(w, tc.response)
				case tc.clusterStatus != "" && req.Method == http.MethodGet && req.URL.Path == clusterPath+"/features":
					_, _ = io.WriteString(w, `{"features":["cimd"]}`)
				case tc.clusterStatus != "" && req.Method == http.MethodGet && req.URL.Path == clusterPath:
					readinessChecks.Add(1)
					_, _ = fmt.Fprintf(w, `{"status":%q}`, tc.clusterStatus)
				default:
					t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
					http.NotFound(w, req)
				}
			}))
			defer server.Close()

			r := &clusterFeaturesResource{client: skycloak.New(server.URL, "unused", "")}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			err := r.replace(ctx, testFeaturesClusterID, []string{"cimd"}, tc.applyNow)
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("replace error = %v, want %q", err, tc.wantError)
			}
			if tc.clusterStatus != "" && readinessChecks.Load() != 1 {
				t.Fatalf("readiness checks = %d, want 1", readinessChecks.Load())
			}
		})
	}
}

func clusterFeaturesConfig(id, flags string, applyNow bool) string {
	return fmt.Sprintf(`
resource "skycloak_cluster_features" "test" {
  cluster_id = %q
  features   = %s
  apply_now  = %t
}
`, id, flags, applyNow)
}

func TestClusterFeaturesResource(t *testing.T) {
	oldInterval := skycloak.ClusterFeaturesPollInterval
	skycloak.ClusterFeaturesPollInterval = time.Millisecond
	t.Cleanup(func() { skycloak.ClusterFeaturesPollInterval = oldInterval })

	type featureRequest struct {
		Features []string `json:"features"`
		ApplyNow bool     `json:"apply_now"`
	}

	var mu sync.Mutex
	flags := []string{}
	change := ""
	applyingReads := 0
	var writes []featureRequest
	clusterPath := "/clusters/" + testFeaturesClusterID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == clusterPath:
			if change != "" {
				t.Errorf("cluster readiness checked during %s", change)
			}
			_, _ = io.WriteString(w, `{"status":"available"}`)
		case req.Method == http.MethodGet && req.URL.Path == clusterPath+"/features":
			if change == "applying" {
				applyingReads++
				if applyingReads == 2 {
					change = ""
				}
			}
		case req.Method == http.MethodPut && req.URL.Path == clusterPath+"/features":
			var input featureRequest
			if err := json.NewDecoder(req.Body).Decode(&input); err != nil || input.Features == nil {
				t.Errorf("expected a complete set of flags: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writes = append(writes, input)
			flags = input.Features
			change = "pending"
			if input.ApplyNow {
				change = "applying"
				applyingReads = 0
			}
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			http.NotFound(w, req)
			return
		}

		if req.URL.Path == clusterPath {
			return
		}
		response := map[string]any{"features": flags, "restarts_keycloak": change != ""}
		if change != "" {
			response["change"] = map[string]string{"state": change}
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	providerConfig := fmt.Sprintf("provider \"skycloak\" {\n  endpoint = %q\n  api_key = \"unused\"\n}\n", server.URL)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if len(writes) != 3 || !writes[2].ApplyNow || len(flags) != 0 || change != "" {
				return fmt.Errorf("destroy must clear the flags and wait for the restart: %v, %v, %q", writes, flags, change)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: providerConfig + clusterFeaturesConfig(testFeaturesClusterID, `["resource-indicators", "cimd"]`, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(clusterFeaturesAddr, "features.#", "2"),
					func(*terraform.State) error {
						mu.Lock()
						defer mu.Unlock()
						if len(writes) != 1 || writes[0].ApplyNow || change != "pending" {
							return fmt.Errorf("expected one scheduled replacement, got %v and %q", writes, change)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      clusterFeaturesAddr,
				ImportState:       true,
				ImportStateId:     testFeaturesClusterID,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					mu.Lock()
					defer mu.Unlock()
					change = ""
				},
				Config: providerConfig + clusterFeaturesConfig(testFeaturesClusterID, `["cimd"]`, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(clusterFeaturesAddr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(clusterFeaturesAddr, "id", testFeaturesClusterID+"/features"),
					resource.TestCheckResourceAttr(clusterFeaturesAddr, "features.#", "1"),
					func(*terraform.State) error {
						mu.Lock()
						defer mu.Unlock()
						if len(writes) != 2 || !writes[1].ApplyNow || !slices.Equal(flags, []string{"cimd"}) || change != "" {
							return fmt.Errorf("update must replace the flags and wait for the restart: %v, %v, %q", writes, flags, change)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccClusterFeaturesResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			clusterID := testAccClusterID(t)
			client := testAccClient()
			cluster, err := client.GetCluster(context.Background(), clusterID)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(cluster.Version, "26.8.") {
				t.Skip("requires a Keycloak 26.8.x cluster")
			}
			flags, err := client.GetClusterFeatures(context.Background(), clusterID)
			if err != nil {
				t.Fatal(err)
			}
			if len(flags.Features) > 0 || flags.Change != "" {
				t.Skip("requires a cluster with no customer-managed flags or pending changes")
			}
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			flags, err := testAccClient().GetClusterFeatures(context.Background(), testAccClusterID(t))
			if err != nil {
				return err
			}
			if len(flags.Features) > 0 || flags.Change != "" {
				return fmt.Errorf("cluster feature flags were not cleared: %+v", flags)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: clusterFeaturesConfig(testAccClusterID(t), `["cimd","resource-indicators"]`, true),
				Check:  resource.TestCheckResourceAttr(clusterFeaturesAddr, "features.#", "2"),
			},
			{
				Config: clusterFeaturesConfig(testAccClusterID(t), `["cimd"]`, true),
				Check:  resource.TestCheckResourceAttr(clusterFeaturesAddr, "features.#", "1"),
			},
			{
				ResourceName:            clusterFeaturesAddr,
				ImportState:             true,
				ImportStateId:           testAccClusterID(t),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"apply_now"},
			},
		},
	})
}
