package provider

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// customThemeSchema returns the resource schema under test.
func customThemeSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	(&customThemeResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// themeStateValue builds a non-null raw object for the custom-theme schema so
// plan modifiers see an existing resource (an update, not a create).
func themeStateValue(t *testing.T, s schema.Schema, m customThemeResourceModel) tftypes.Value {
	t.Helper()
	st := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
	if diags := st.Set(context.Background(), m); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	return st.Raw
}

// planModifiesRequiresReplace runs every string plan modifier on an attribute
// for a state→plan value change and reports whether any of them asks Terraform
// to destroy and recreate the resource.
func planModifiesRequiresReplace(t *testing.T, s schema.Schema, attr, stateVal, planVal string) bool {
	t.Helper()
	ctx := context.Background()
	a, ok := s.Attributes[attr].(schema.StringAttribute)
	if !ok {
		t.Fatalf("attribute %q is not a string attribute", attr)
	}
	stateModel := customThemeResourceModel{
		ID:         types.StringValue("44444444-4444-4444-4444-444444444444"),
		ClusterID:  types.StringValue("11111111-1111-1111-1111-111111111111"),
		Source:     types.StringValue("theme.zip"),
		Name:       types.StringValue("corp"),
		ThemeTypes: types.ListNull(types.StringType),
	}
	raw := themeStateValue(t, s, stateModel)
	req := planmodifier.StringRequest{
		Path:        path.Root(attr),
		StateValue:  types.StringValue(stateVal),
		PlanValue:   types.StringValue(planVal),
		ConfigValue: types.StringNull(),
		State:       tfsdk.State{Schema: s, Raw: raw},
		Plan:        tfsdk.Plan{Schema: s, Raw: raw},
		Config:      tfsdk.Config{Schema: s, Raw: raw},
	}
	for _, m := range a.PlanModifiers {
		var resp planmodifier.StringResponse
		resp.PlanValue = req.PlanValue
		m.PlanModifyString(ctx, req, &resp)
		if resp.RequiresReplace {
			return true
		}
	}
	return false
}

// themeAPIRecorder is a fake Skycloak API that records which theme endpoints an
// apply touches, so a test can assert what the provider did *not* call
// (delete, re-upload, or a realm assignment) as well as what it did.
type themeAPIRecorder struct {
	calls      []string
	contentPut int
	metadata   int
	version    string
	fileBody   string
	// metaVersion is the version field of the last metadata request, nil when
	// the request left it out entirely.
	metaVersion *string

	// name and storedVersion are the theme as the fake currently holds it, so a
	// label the metadata endpoint clears stays cleared on the later reads.
	name          string
	storedVersion string
}

func (rec *themeAPIRecorder) server(t *testing.T, clusterID, themeID string) *httptest.Server {
	t.Helper()
	base := "/clusters/" + clusterID + "/themes"
	// The fake starts out holding the theme testThemeModel describes.
	rec.name, rec.storedVersion = "corp", "1.0.0"
	theme := func(status string) string {
		v := "null"
		if rec.storedVersion != "" {
			v = `"` + rec.storedVersion + `"`
		}
		return `{"id":"` + themeID + `","cluster_id":"` + clusterID + `","name":"` + rec.name + `","version":` + v +
			`,"status":"` + status + `","theme_types":["login"],"file_size":7,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}`
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.calls = append(rec.calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == base+"/"+themeID+"/content":
			rec.contentPut++
			fields, _ := parseThemeMultipart(t, r)
			rec.version = strings.Join(fields["version"], "")
			rec.fileBody = strings.Join(fields["theme_file"], "")
			// An omitted version keeps the stored label, per the API contract.
			if rec.version != "" {
				rec.storedVersion = rec.version
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(theme("deploying")))
		case r.Method == http.MethodPatch && r.URL.Path == base+"/"+themeID:
			rec.metadata++
			var body struct {
				Name    *string `json:"name"`
				Version *string `json:"version"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode metadata body: %v", err)
			}
			rec.metaVersion = body.Version
			if body.Name != nil {
				rec.name = *body.Name
			}
			if body.Version != nil {
				rec.storedVersion = *body.Version
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(theme("deployed")))
		case r.Method == http.MethodGet && r.URL.Path == base+"/"+themeID:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(theme("deployed")))
		default:
			http.NotFound(w, r)
		}
	}))
}

func (rec *themeAPIRecorder) called(method string) bool {
	for _, c := range rec.calls {
		if strings.HasPrefix(c, method+" ") {
			return true
		}
	}
	return false
}

// parseThemeMultipart reads a multipart request body into field→values.
func parseThemeMultipart(t *testing.T, r *http.Request) (map[string][]string, map[string]string) {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse content-type: %v", err)
	}
	mr := multipart.NewReader(r.Body, params["boundary"])
	fields := map[string][]string{}
	fileCT := map[string]string{}
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		body, _ := io.ReadAll(p)
		if p.FileName() != "" {
			fileCT[p.FormName()] = p.Header.Get("Content-Type")
		}
		fields[p.FormName()] = append(fields[p.FormName()], string(body))
	}
	return fields, fileCT
}

// runThemeUpdate drives the resource's Update against a fake API and returns
// the state it wrote.
func runThemeUpdate(t *testing.T, endpoint string, state, plan customThemeResourceModel) (customThemeResourceModel, diag.Diagnostics) {
	t.Helper()
	ctx := context.Background()
	s := customThemeSchema(t)
	r := &customThemeResource{client: skycloak.New(endpoint, "sk_sc_test_aaa_bbb", "")}

	req := resource.UpdateRequest{
		State: tfsdk.State{Schema: s, Raw: themeStateValue(t, s, state)},
		Plan:  tfsdk.Plan{Schema: s, Raw: themeStateValue(t, s, plan)},
	}
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: themeStateValue(t, s, state)}}
	r.Update(ctx, req, &resp)

	var out customThemeResourceModel
	if !resp.State.Raw.IsNull() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &out)...)
	}
	return out, resp.Diagnostics
}

// themeArchive writes a stand-in theme archive and returns its path and hash.
func themeArchive(t *testing.T, name, content string) (string, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	sum, err := fileSHA256(p)
	if err != nil {
		t.Fatalf("hash archive: %v", err)
	}
	return p, sum
}

const (
	testThemeClusterID = "11111111-1111-1111-1111-111111111111"
	testThemeID        = "44444444-4444-4444-4444-444444444444"
)

func testThemeModel(source, sha string) customThemeResourceModel {
	return customThemeResourceModel{
		ID:            types.StringValue(testThemeID),
		ClusterID:     types.StringValue(testThemeClusterID),
		Source:        types.StringValue(source),
		ContentSHA256: types.StringValue(sha),
		Name:          types.StringValue("corp"),
		Description:   types.StringNull(),
		Version:       types.StringValue("1.0.0"),
		ThemeTypes:    types.ListNull(types.StringType),
		Status:        types.StringValue("deployed"),
		FileSize:      types.Int64Value(7),
		DeployedAt:    types.StringValue("2026-01-01T00:00:00Z"),
	}
}

// TestCustomThemeUpdateContentInPlace is the apply-level guarantee: a
// content-only change replaces the archive through the content endpoint,
// keeping the theme's ID and name, and never deletes, re-uploads, or touches a
// realm assignment.
func TestCustomThemeUpdateContentInPlace(t *testing.T) {
	rec := &themeAPIRecorder{}
	srv := rec.server(t, testThemeClusterID, testThemeID)
	defer srv.Close()

	oldPath, oldSHA := themeArchive(t, "corp.zip", "old-bytes")
	newPath, newSHA := themeArchive(t, "corp.zip", "new-bytes")

	state := testThemeModel(oldPath, oldSHA)
	plan := testThemeModel(newPath, newSHA)

	got, diags := runThemeUpdate(t, srv.URL, state, plan)
	if diags.HasError() {
		t.Fatalf("update diagnostics: %v", diags)
	}
	if rec.contentPut != 1 {
		t.Errorf("content endpoint called %d times, want 1 (calls: %v)", rec.contentPut, rec.calls)
	}
	if rec.fileBody != "new-bytes" {
		t.Errorf("uploaded body = %q, want the new archive bytes", rec.fileBody)
	}
	if rec.called(http.MethodDelete) || rec.called(http.MethodPost) {
		t.Errorf("content update must not delete or re-upload the theme (calls: %v)", rec.calls)
	}
	if rec.metadata != 0 {
		t.Errorf("metadata endpoint called %d times for a content-only change, want 0", rec.metadata)
	}
	if got.ID.ValueString() != testThemeID {
		t.Errorf("id = %q, want the theme's id preserved", got.ID.ValueString())
	}
	if got.Name.ValueString() != "corp" {
		t.Errorf("name = %q, want corp preserved", got.Name.ValueString())
	}
	if got.ContentSHA256.ValueString() != newSHA {
		t.Errorf("content_sha256 = %q, want the new archive hash", got.ContentSHA256.ValueString())
	}
	if got.Status.ValueString() != "deployed" {
		t.Errorf("status = %q, want the update to wait for deployed", got.Status.ValueString())
	}
}

// TestCustomThemeUpdateMetadataOnly keeps the cheap path cheap: renaming a
// theme must not re-upload its archive.
func TestCustomThemeUpdateMetadataOnly(t *testing.T) {
	rec := &themeAPIRecorder{}
	srv := rec.server(t, testThemeClusterID, testThemeID)
	defer srv.Close()

	sourcePath, sha := themeArchive(t, "corp.zip", "same-bytes")
	state := testThemeModel(sourcePath, sha)
	plan := testThemeModel(sourcePath, sha)
	plan.Name = types.StringValue("corp-renamed")

	got, diags := runThemeUpdate(t, srv.URL, state, plan)
	if diags.HasError() {
		t.Fatalf("update diagnostics: %v", diags)
	}
	if rec.contentPut != 0 {
		t.Errorf("content endpoint called %d times for a rename, want 0", rec.contentPut)
	}
	if rec.metadata != 1 {
		t.Errorf("metadata endpoint called %d times, want 1 (calls: %v)", rec.metadata, rec.calls)
	}
	if got.Name.ValueString() != "corp-renamed" {
		t.Errorf("name = %q, want corp-renamed", got.Name.ValueString())
	}
}

// TestCustomThemeUpdateContentAndMetadata covers both changing at once: one
// metadata call plus one content call, still without a destroy/create.
func TestCustomThemeUpdateContentAndMetadata(t *testing.T) {
	rec := &themeAPIRecorder{}
	srv := rec.server(t, testThemeClusterID, testThemeID)
	defer srv.Close()

	oldPath, oldSHA := themeArchive(t, "corp.zip", "old-bytes")
	newPath, newSHA := themeArchive(t, "corp.zip", "new-bytes")
	state := testThemeModel(oldPath, oldSHA)
	plan := testThemeModel(newPath, newSHA)
	plan.Name = types.StringValue("corp-renamed")
	plan.Version = types.StringValue("2.0.0")

	got, diags := runThemeUpdate(t, srv.URL, state, plan)
	if diags.HasError() {
		t.Fatalf("update diagnostics: %v", diags)
	}
	if rec.metadata != 1 || rec.contentPut != 1 {
		t.Errorf("calls = %v, want one metadata update and one content update", rec.calls)
	}
	if rec.version != "2.0.0" {
		t.Errorf("content update version = %q, want the planned 2.0.0", rec.version)
	}
	if rec.called(http.MethodDelete) || rec.called(http.MethodPost) {
		t.Errorf("update must not delete or re-upload the theme (calls: %v)", rec.calls)
	}
	if got.ID.ValueString() != testThemeID {
		t.Errorf("id = %q, want the theme's id preserved", got.ID.ValueString())
	}
	if got.Name.ValueString() != "corp-renamed" {
		t.Errorf("name = %q, want corp-renamed", got.Name.ValueString())
	}
}

// TestCustomThemeUpdateRemovesVersionWithContent covers dropping `version` from
// the configuration in the same apply that changes the archive. The content
// request cannot carry the removal (an empty version means "keep the current
// label"), so the metadata endpoint has to clear it, otherwise the old label
// comes back into state and the apply reports an inconsistent result.
func TestCustomThemeUpdateRemovesVersionWithContent(t *testing.T) {
	rec := &themeAPIRecorder{}
	srv := rec.server(t, testThemeClusterID, testThemeID)
	defer srv.Close()

	oldPath, oldSHA := themeArchive(t, "corp.zip", "old-bytes")
	newPath, newSHA := themeArchive(t, "corp.zip", "new-bytes")
	state := testThemeModel(oldPath, oldSHA)
	plan := testThemeModel(newPath, newSHA)
	plan.Version = types.StringNull()

	got, diags := runThemeUpdate(t, srv.URL, state, plan)
	if diags.HasError() {
		t.Fatalf("update diagnostics: %v", diags)
	}
	if rec.metadata != 1 {
		t.Errorf("metadata endpoint called %d times, want 1 to clear the version (calls: %v)", rec.metadata, rec.calls)
	}
	if rec.metaVersion == nil || *rec.metaVersion != "" {
		t.Errorf("metadata version field = %v, want an explicit empty string", rec.metaVersion)
	}
	if rec.contentPut != 1 {
		t.Errorf("content endpoint called %d times, want 1", rec.contentPut)
	}
	if !got.Version.IsNull() {
		t.Errorf("version = %q, want the configured removal to hold", got.Version.ValueString())
	}
}

// TestCustomThemeUpdateRemovesVersionOnly is the same removal without a content
// change: the metadata endpoint still has to clear the label.
func TestCustomThemeUpdateRemovesVersionOnly(t *testing.T) {
	rec := &themeAPIRecorder{}
	srv := rec.server(t, testThemeClusterID, testThemeID)
	defer srv.Close()

	sourcePath, sha := themeArchive(t, "corp.zip", "same-bytes")
	state := testThemeModel(sourcePath, sha)
	plan := testThemeModel(sourcePath, sha)
	plan.Version = types.StringNull()

	got, diags := runThemeUpdate(t, srv.URL, state, plan)
	if diags.HasError() {
		t.Fatalf("update diagnostics: %v", diags)
	}
	if rec.metadata != 1 || rec.contentPut != 0 {
		t.Errorf("calls = %v, want one metadata update and no content update", rec.calls)
	}
	if !got.Version.IsNull() {
		t.Errorf("version = %q, want the configured removal to hold", got.Version.ValueString())
	}
}

// TestCustomThemeContentChangePlansInPlace is the plan-level guarantee of the
// resource: new archive bytes (a changed content_sha256) update the theme in
// place, while the cluster it lives on is still immutable.
func TestCustomThemeContentChangePlansInPlace(t *testing.T) {
	s := customThemeSchema(t)

	if planModifiesRequiresReplace(t, s, "content_sha256", "aaa", "bbb") {
		t.Error("a content_sha256 change still forces replacement; content updates must be in place")
	}
	if planModifiesRequiresReplace(t, s, "source", "a.zip", "b.zip") {
		t.Error("a source change still forces replacement")
	}
	if !planModifiesRequiresReplace(t, s, "cluster_id", "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222") {
		t.Error("cluster_id must still force replacement: a theme cannot move between clusters")
	}
}
