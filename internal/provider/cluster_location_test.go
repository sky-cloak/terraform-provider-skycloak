package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestReconcileClusterLocation covers the Read-side half of the us-west guard:
// a cluster recorded as `us` that the API now reports as `us-west` keeps `us`
// in state, with a warning, while every other API value is taken as reported.
func TestReconcileClusterLocation(t *testing.T) {
	cases := []struct {
		name, prior, api, want string
		warn                   bool
	}{
		{"recorded us, reported us-west", "us", "us-west", "us", true},
		{"recorded us, reported us", "us", "us", "us", false},
		{"recorded us-west, reported us-west", "us-west", "us-west", "us-west", false},
		{"import (nothing recorded), reported us-west", "", "us-west", "us-west", false},
		{"import (nothing recorded), reported us", "", "us", "us", false},
		{"recorded eu, reported eu", "eu", "eu", "eu", false},
		{"recorded ca, reported us-west", "ca", "us-west", "us-west", false},
		{"recorded us-west, reported us", "us-west", "us", "us", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := reconcileClusterLocation(tc.prior, tc.api, &diags)
			if got != tc.want {
				t.Errorf("location = %q, want %q", got, tc.want)
			}
			if diags.HasError() {
				t.Fatalf("unexpected error diagnostics: %v", diags)
			}
			if gotWarn := diags.WarningsCount() > 0; gotWarn != tc.warn {
				t.Fatalf("warning emitted = %v, want %v (%v)", gotWarn, tc.warn, diags)
			}
			if tc.warn && !strings.Contains(diags.Warnings()[0].Detail(), `location = "us-west"`) {
				t.Errorf("warning must tell the user what to write, got: %s", diags.Warnings()[0].Detail())
			}
		})
	}
}

// TestClusterLocationChangeRequiresReplace covers the plan-side half: only a
// relabel between us and us-west of a cluster the API already reports as
// us-west is exempt from replacement. Every other change is a real move.
func TestClusterLocationChangeRequiresReplace(t *testing.T) {
	cases := []struct {
		name, state, config, api string
		want                     bool
	}{
		{"relabel us to us-west, West cluster", "us", "us-west", "us-west", false},
		{"relabel us-west to us, West cluster", "us-west", "us", "us-west", false},
		{"us to us-west, East cluster", "us", "us-west", "us", true},
		{"us to us-west, API location never recorded", "us", "us-west", "", true},
		{"us-west to us, East cluster", "us-west", "us", "us", true},
		{"us to eu", "us", "eu", "us", true},
		{"us, West cluster, to eu", "us", "eu", "us-west", true},
		{"us-west to ca", "us-west", "ca", "us-west", true},
		{"eu to us", "eu", "us", "eu", true},
		{"au to us-west, API reports us-west", "au", "us-west", "us-west", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clusterLocationChangeRequiresReplace(tc.state, tc.config, tc.api); got != tc.want {
				t.Errorf("requires replace = %v, want %v", got, tc.want)
			}
		})
	}
}

const testLocationClusterID = "22222222-2222-2222-2222-222222222222"

// clusterLocationAPI is a fake Skycloak cluster API whose reported location a
// test can switch between steps, standing in for the moment the public API
// starts reporting a West Coast cluster as us-west instead of us. It records
// every call that could change a cluster.
type clusterLocationAPI struct {
	mu        sync.Mutex
	exists    bool
	name      string
	size      string
	location  string
	mutations []string
}

func newClusterLocationAPI(t *testing.T, location string, exists bool) (*clusterLocationAPI, *httptest.Server) {
	t.Helper()
	api := &clusterLocationAPI{exists: exists, name: "tf-location", size: "small", location: location}
	srv := httptest.NewServer(http.HandlerFunc(api.serve))
	t.Cleanup(srv.Close)
	return api, srv
}

func (a *clusterLocationAPI) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Method != http.MethodGet {
		a.mutations = append(a.mutations, r.Method+" "+r.URL.Path)
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/clusters":
		var body struct {
			Name     string `json:"name"`
			Size     string `json:"size"`
			Location string `json:"location"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		a.exists, a.name, a.size, a.location = true, body.Name, body.Size, body.Location
		a.write(w, http.StatusCreated)
	case r.URL.Path == "/clusters/"+testLocationClusterID && a.exists:
		switch r.Method {
		case http.MethodGet:
			a.write(w, http.StatusOK)
		case http.MethodPatch:
			var body struct {
				Size *string `json:"size"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Size != nil {
				a.size = *body.Size
			}
			a.write(w, http.StatusOK)
		case http.MethodDelete:
			a.exists = false
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	default:
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"title":"not found","status":404}`))
	}
}

func (a *clusterLocationAPI) write(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"id":%q,"name":%q,"type":"keycloak","size":%q,"version":"26.1","location":%q,"status":"available","url":"https://example.test","auto_upgrade_enabled":false,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
		testLocationClusterID, a.name, a.size, a.location)
}

// report switches the location the fake API reports for the cluster.
func (a *clusterLocationAPI) report(location string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.location = location
}

// resetMutations forgets the calls recorded so far, so a step can assert on
// only the calls it made itself.
func (a *clusterLocationAPI) resetMutations() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mutations = nil
}

// checkMutations asserts the calls that could change the cluster since the
// last reset, in order.
func (a *clusterLocationAPI) checkMutations(want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if strings.Join(a.mutations, ",") != strings.Join(want, ",") {
			return fmt.Errorf("mutating API calls = %v, want %v", a.mutations, want)
		}
		return nil
	}
}

func clusterLocationConfig(endpoint, size, location string) string {
	return fmt.Sprintf(`
provider "skycloak" {
  endpoint = %q
  api_key  = "sk_sc_test_aaa_bbb"
}

resource "skycloak_cluster" "test" {
  name     = "tf-location"
  type     = "keycloak"
  size     = %q
  version  = "26.1"
  location = %q
}
`, endpoint, size, location)
}

const clusterAddr = "skycloak_cluster.test"

var patchCluster = http.MethodPatch + " /clusters/" + testLocationClusterID

// TestClusterLocationExistingUSStateNoDiff: a cluster whose state and config
// say `us` but which the API now reports as `us-west` plans with no diff and no
// replacement, and a later unrelated change still applies cleanly.
func TestClusterLocationExistingUSStateNoDiff(t *testing.T) {
	api, srv := newClusterLocationAPI(t, "us", false)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Today's API reports the West Coast cluster as us.
				Config: clusterLocationConfig(srv.URL, "small", "us"),
				Check:  resource.TestCheckResourceAttr(clusterAddr, "location", "us"),
			},
			{
				PreConfig: func() { api.report("us-west"); api.resetMutations() },
				Config:    clusterLocationConfig(srv.URL, "small", "us"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(clusterAddr, "location", "us"),
					api.checkMutations(),
				),
			},
			{
				// An unrelated update must not leak us-west into state, which
				// Terraform would reject as an inconsistent result.
				PreConfig: api.resetMutations,
				Config:    clusterLocationConfig(srv.URL, "medium", "us"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(clusterAddr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(clusterAddr, "location", "us"),
					resource.TestCheckResourceAttr(clusterAddr, "size", "medium"),
					api.checkMutations(patchCluster),
				),
			},
		},
	})
}

// TestClusterLocationMigrateConfigToUSWest: moving that cluster's config from
// `us` to `us-west` is an in-place relabel that makes no API call able to
// change the cluster.
func TestClusterLocationMigrateConfigToUSWest(t *testing.T) {
	api, srv := newClusterLocationAPI(t, "us", false)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: clusterLocationConfig(srv.URL, "small", "us")},
			{
				PreConfig: func() { api.report("us-west"); api.resetMutations() },
				Config:    clusterLocationConfig(srv.URL, "small", "us-west"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(clusterAddr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(clusterAddr, "location", "us-west"),
					resource.TestCheckResourceAttr(clusterAddr, "id", testLocationClusterID),
					api.checkMutations(),
				),
			},
		},
	})
}

// TestClusterLocationImportWestCluster: importing a cluster the API reports as
// us-west plans no replacement whether the config says `us` or `us-west`.
func TestClusterLocationImportWestCluster(t *testing.T) {
	cases := []struct {
		config string
		action plancheck.PlanCheck
	}{
		{"us-west", plancheck.ExpectEmptyPlan()},
		{"us", plancheck.ExpectResourceAction(clusterAddr, plancheck.ResourceActionUpdate)},
	}
	for _, tc := range cases {
		t.Run("config "+tc.config, func(t *testing.T) {
			api, srv := newClusterLocationAPI(t, "us-west", true)
			cfg := clusterLocationConfig(srv.URL, "small", tc.config)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:             cfg,
						ResourceName:       clusterAddr,
						ImportState:        true,
						ImportStateId:      testLocationClusterID,
						ImportStatePersist: true,
					},
					{
						PreConfig: api.resetMutations,
						Config:    cfg,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{tc.action},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(clusterAddr, "location", tc.config),
							api.checkMutations(),
						),
					},
				},
			})
		})
	}
}

// TestClusterLocationRealMovesReplace: the guard never suppresses a genuine
// move. An East cluster relabelled us-west, and any change between the US
// labels and another region, still plans a replacement.
func TestClusterLocationRealMovesReplace(t *testing.T) {
	cases := []struct {
		name, reported, from, to string
	}{
		{"East cluster to us-west", "us", "us", "us-west"},
		{"West cluster recorded as us, to eu", "us-west", "us", "eu"},
		{"us-west to ca", "us-west", "us-west", "ca"},
		{"au to us-west", "au", "au", "us-west"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, srv := newClusterLocationAPI(t, tc.from, false)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: clusterLocationConfig(srv.URL, "small", tc.from)},
					{
						PreConfig: func() { api.report(tc.reported) },
						Config:    clusterLocationConfig(srv.URL, "small", tc.to),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(clusterAddr, plancheck.ResourceActionDestroyBeforeCreate)},
						},
						Check: resource.TestCheckResourceAttr(clusterAddr, "location", tc.to),
					},
				},
			})
		})
	}
}

// TestClusterLocationPlainUSNoDiff: an East cluster configured as `us` shows no
// diff, whether or not the API has started reporting us-west for West clusters.
func TestClusterLocationPlainUSNoDiff(t *testing.T) {
	_, srv := newClusterLocationAPI(t, "us", false)
	cfg := clusterLocationConfig(srv.URL, "small", "us")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(clusterAddr, "location", "us"),
			},
		},
	})
}
