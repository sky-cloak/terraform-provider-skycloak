package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// fakeThemeAPI is a minimal theme API whose file_size and deployed_at follow
// the bytes it was last given, as the real API's do. A fixed file_size would
// hide a plan that disagrees with what the API reports after an update.
type fakeThemeAPI struct {
	mu         sync.Mutex
	clusterID  string
	themeID    string
	exists     bool
	name       string
	version    string
	size       int
	deployedAt time.Time
	uploads    int
}

func (f *fakeThemeAPI) store(fields map[string][]string, name string) {
	f.size = len(strings.Join(fields["theme_file"], ""))
	f.deployedAt = f.deployedAt.Add(time.Hour)
	f.uploads++
	if v := strings.Join(fields["version"], ""); v != "" {
		f.version = v
	}
	if name != "" {
		f.name = name
	}
}

func (f *fakeThemeAPI) body() string {
	version := "null"
	if f.version != "" {
		version = strconv.Quote(f.version)
	}
	return fmt.Sprintf(`{"id":%q,"cluster_id":%q,"name":%q,"version":%s,"status":"deployed","theme_types":["login"],`+
		`"file_size":%d,"deployed_at":%q,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}`,
		f.themeID, f.clusterID, f.name, version, f.size, f.deployedAt.Format(time.RFC3339))
}

func (f *fakeThemeAPI) server(t *testing.T) *httptest.Server {
	base := "/clusters/" + f.clusterID + "/themes"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			fields, _ := parseThemeMultipart(t, r)
			f.exists = true
			f.store(fields, strings.Join(fields["name"], ""))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(f.body()))
		case f.exists && r.Method == http.MethodPut && r.URL.Path == base+"/"+f.themeID+"/content":
			fields, _ := parseThemeMultipart(t, r)
			f.store(fields, "")
			_, _ = w.Write([]byte(f.body()))
		case f.exists && r.Method == http.MethodPatch && r.URL.Path == base+"/"+f.themeID:
			var in struct {
				Name    *string `json:"name"`
				Version *string `json:"version"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Errorf("decode metadata body: %v", err)
			}
			if in.Name != nil {
				f.name = *in.Name
			}
			if in.Version != nil {
				f.version = *in.Version
			}
			_, _ = w.Write([]byte(f.body()))
		case f.exists && r.Method == http.MethodGet && r.URL.Path == base+"/"+f.themeID:
			_, _ = w.Write([]byte(f.body()))
		case f.exists && r.Method == http.MethodDelete && r.URL.Path == base+"/"+f.themeID:
			f.exists = false
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
}

// TestCustomThemeResourceContentUpdate runs through Terraform core, which
// checks the applied result against the plan. Changing the archive bytes at an
// unchanged path must plan file_size, deployed_at and status as unknown, so
// the values the API returns after the update are accepted.
func TestCustomThemeResourceContentUpdate(t *testing.T) {
	api := &fakeThemeAPI{
		clusterID:  testThemeClusterID,
		themeID:    testThemeID,
		deployedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	srv := api.server(t)
	defer srv.Close()

	archive := filepath.Join(t.TempDir(), "theme.zip")
	writeThemeZip(t, archive, "corp", "v1")
	moved := filepath.Join(t.TempDir(), "moved.zip")

	config := func(source, version string) string {
		return fmt.Sprintf(`
provider "skycloak" {
  endpoint = %q
  api_key  = "sk_sc_test_aaa_bbb"
}

resource "skycloak_custom_theme" "test" {
  cluster_id = %q
  source     = %q
  name       = "corp"
  version    = %q
}`, srv.URL, testThemeClusterID, source, version)
	}
	res := "skycloak_custom_theme.test"
	var firstSize, firstDeployedAt string

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(archive, "1.0.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith(res, "file_size", func(v string) error { firstSize = v; return nil }),
					resource.TestCheckResourceAttrWith(res, "deployed_at", func(v string) error { firstDeployedAt = v; return nil }),
				),
			},
			{
				// Nothing changed: a plan must not show a diff.
				Config:             config(archive, "1.0.0"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				// Same config, new archive bytes.
				PreConfig: func() { writeThemeZip(t, archive, "corp", "v2 with a longer marker to change the size") },
				Config:    config(archive, "1.0.0"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(res, plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(res, tfjsonpath.New("file_size")),
						plancheck.ExpectUnknownValue(res, tfjsonpath.New("deployed_at")),
						plancheck.ExpectUnknownValue(res, tfjsonpath.New("status")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						api.mu.Lock()
						defer api.mu.Unlock()
						return resource.TestCheckResourceAttr(res, "file_size", strconv.Itoa(api.size))(s)
					},
					resource.TestCheckResourceAttr(res, "status", "deployed"),
					resource.TestCheckResourceAttrWith(res, "file_size", func(v string) error {
						if v == firstSize {
							return fmt.Errorf("file_size = %s, want the size of the new archive", v)
						}
						return nil
					}),
					resource.TestCheckResourceAttrWith(res, "deployed_at", func(v string) error {
						if v == firstDeployedAt {
							return fmt.Errorf("deployed_at = %s, want the new deployment time", v)
						}
						return nil
					}),
				),
			},
			{
				Config:             config(archive, "1.0.0"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				// Only the version label changes.
				Config: config(archive, "1.1.0"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(res, plancheck.ResourceActionUpdate)},
				},
				Check: resource.TestCheckResourceAttr(res, "version", "1.1.0"),
			},
			{
				// Same bytes at a new path: applies cleanly.
				PreConfig: func() { copyThemeFile(t, archive, moved) },
				Config:    config(moved, "1.1.0"),
				Check:     resource.TestCheckResourceAttr(res, "source", moved),
			},
		},
	})
}

func copyThemeFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatalf("read %s: %v", from, err)
	}
	if err := os.WriteFile(to, b, 0o600); err != nil {
		t.Fatalf("write %s: %v", to, err)
	}
}
