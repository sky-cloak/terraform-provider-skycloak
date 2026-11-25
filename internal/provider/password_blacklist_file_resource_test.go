package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

const testBlacklistClusterID = "11111111-1111-1111-1111-111111111111"

func blacklistSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	(&passwordBlacklistFileResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func blacklistRaw(t *testing.T, s schema.Schema, m passwordBlacklistFileResourceModel) tftypes.Value {
	t.Helper()
	st := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
	if diags := st.Set(context.Background(), m); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	return st.Raw
}

func blacklistModel(source, name, sha string) passwordBlacklistFileResourceModel {
	return passwordBlacklistFileResourceModel{
		ID:            types.StringValue(name),
		ClusterID:     types.StringValue(testBlacklistClusterID),
		Name:          types.StringValue(name),
		Source:        types.StringValue(source),
		ApplyNow:      types.BoolValue(false),
		ContentSHA256: types.StringValue(sha),
		Size:          types.Int64Value(7),
		Status:        types.StringValue("ready"),
	}
}

func blacklistFile(t *testing.T, content string) (string, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "list.txt")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA256(p)
	if err != nil {
		t.Fatal(err)
	}
	return p, sum
}

// TestPasswordBlacklistFileSchemaReplacement pins what forces a new file:
// the cluster and the name, and nothing else.
func TestPasswordBlacklistFileSchemaReplacement(t *testing.T) {
	s := blacklistSchema(t)
	for attr, want := range map[string]bool{"cluster_id": true, "name": true, "source": false, "apply_now": false} {
		var got bool
		switch a := s.Attributes[attr].(type) {
		case schema.StringAttribute:
			for _, m := range a.PlanModifiers {
				if strings.Contains(m.Description(context.Background()), "destroy and recreate") || strings.Contains(m.Description(context.Background()), "replace") {
					got = true
				}
			}
		case schema.BoolAttribute:
			got = len(a.PlanModifiers) > 0
		}
		if got != want {
			t.Errorf("%s requires replace = %v, want %v", attr, got, want)
		}
	}
}

func runBlacklistModifyPlan(t *testing.T, state *passwordBlacklistFileResourceModel, plan passwordBlacklistFileResourceModel) resource.ModifyPlanResponse {
	t.Helper()
	s := blacklistSchema(t)
	req := resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: blacklistRaw(t, s, plan)},
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)},
	}
	if state != nil {
		req.State.Raw = blacklistRaw(t, s, *state)
	}
	resp := resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: s, Raw: req.Plan.Raw}}
	(&passwordBlacklistFileResource{}).ModifyPlan(context.Background(), req, &resp)
	return resp
}

func TestPasswordBlacklistFileModifyPlan(t *testing.T) {
	oldPath, oldSHA := blacklistFile(t, "old\n")
	newPath, newSHA := blacklistFile(t, "new\n")

	t.Run("create hashes the file", func(t *testing.T) {
		resp := runBlacklistModifyPlan(t, nil, blacklistModel(newPath, "common", ""))
		if resp.Diagnostics.HasError() {
			t.Fatalf("diagnostics: %v", resp.Diagnostics)
		}
		var got passwordBlacklistFileResourceModel
		resp.Plan.Get(context.Background(), &got)
		if got.ContentSHA256.ValueString() != newSHA {
			t.Errorf("content_sha256 = %q, want %q", got.ContentSHA256.ValueString(), newSHA)
		}
	})

	t.Run("same content is no change", func(t *testing.T) {
		state := blacklistModel(oldPath, "common", oldSHA)
		if resp := runBlacklistModifyPlan(t, &state, blacklistModel(oldPath, "common", oldSHA)); resp.Diagnostics.HasError() {
			t.Fatalf("diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("new content under the same name is refused", func(t *testing.T) {
		state := blacklistModel(oldPath, "common", oldSHA)
		resp := runBlacklistModifyPlan(t, &state, blacklistModel(newPath, "common", oldSHA))
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error for changed content under an unchanged name")
		}
		if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "new name") {
			t.Errorf("detail = %q, want it to ask for a new name", resp.Diagnostics.Errors()[0].Detail())
		}
	})

	t.Run("new content under a new name is a replacement", func(t *testing.T) {
		state := blacklistModel(oldPath, "common", oldSHA)
		if resp := runBlacklistModifyPlan(t, &state, blacklistModel(newPath, "common-2", oldSHA)); resp.Diagnostics.HasError() {
			t.Fatalf("diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("missing file is reported", func(t *testing.T) {
		resp := runBlacklistModifyPlan(t, nil, blacklistModel(filepath.Join(t.TempDir(), "absent.txt"), "common", ""))
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error for an unreadable file")
		}
	})
}

type blacklistAPI struct {
	calls    []string
	statuses []string // status returned by successive GETs; the last repeats
	gets     int
	applyNow string
	fileBody string
	sha      string
	delete   int
}

func (a *blacklistAPI) server(t *testing.T) *httptest.Server {
	t.Helper()
	base := "/clusters/" + testBlacklistClusterID + "/password-blacklist-files"
	file := func(status string) string {
		return `{"name":"common","size":4,"sha256":"` + a.sha + `","status":"` + status + `","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.calls = append(a.calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			fields, _ := parseThemeMultipart(t, r)
			a.applyNow = strings.Join(fields["apply_now"], "")
			a.fileBody = strings.Join(fields["file"], "")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(file("waiting_for_restart")))
		case r.Method == http.MethodGet && r.URL.Path == base+"/common":
			s := a.statuses[min(a.gets, len(a.statuses)-1)]
			a.gets++
			_, _ = w.Write([]byte(file(s)))
		case r.Method == http.MethodDelete && r.URL.Path == base+"/common":
			a.delete++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
}

func runBlacklistCreate(t *testing.T, endpoint string, plan passwordBlacklistFileResourceModel) (passwordBlacklistFileResourceModel, resource.CreateResponse) {
	t.Helper()
	s := blacklistSchema(t)
	r := &passwordBlacklistFileResource{client: skycloak.New(endpoint, "sk_sc_test_aaa_bbb", "")}
	req := resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: blacklistRaw(t, s, plan)}}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}}
	r.Create(context.Background(), req, &resp)
	var out passwordBlacklistFileResourceModel
	if !resp.State.Raw.IsNull() {
		resp.State.Get(context.Background(), &out)
	}
	return out, resp
}

func unknownComputed(m passwordBlacklistFileResourceModel) passwordBlacklistFileResourceModel {
	m.ID, m.ContentSHA256, m.Size, m.Status = types.StringUnknown(), types.StringUnknown(), types.Int64Unknown(), types.StringUnknown()
	return m
}

func TestPasswordBlacklistFileCreateWithoutApplyNow(t *testing.T) {
	p, sum := blacklistFile(t, "pass")
	api := &blacklistAPI{statuses: []string{"waiting_for_restart"}, sha: sum}
	srv := api.server(t)
	defer srv.Close()

	plan := unknownComputed(blacklistModel(p, "common", ""))
	plan.ContentSHA256 = types.StringValue(sum)
	got, resp := runBlacklistCreate(t, srv.URL, plan)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if api.applyNow != "" || api.fileBody != "pass" {
		t.Errorf("apply_now = %q, body = %q", api.applyNow, api.fileBody)
	}
	if got.Status.ValueString() != "waiting_for_restart" || got.ID.ValueString() != "common" || got.Size.ValueInt64() != 4 {
		t.Errorf("state = %+v", got)
	}
}

func TestPasswordBlacklistFileCreateApplyNowWaitsForReady(t *testing.T) {
	p, sum := blacklistFile(t, "pass")
	api := &blacklistAPI{statuses: []string{"delivering", "ready"}, sha: sum}
	srv := api.server(t)
	defer srv.Close()

	plan := unknownComputed(blacklistModel(p, "common", ""))
	plan.ApplyNow = types.BoolValue(true)
	plan.ContentSHA256 = types.StringValue(sum)
	got, resp := runBlacklistCreate(t, srv.URL, plan)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if api.applyNow != "true" {
		t.Errorf("apply_now sent = %q, want true", api.applyNow)
	}
	if got.Status.ValueString() != "ready" {
		t.Errorf("status = %q, want ready", got.Status.ValueString())
	}
}

func TestPasswordBlacklistFileCreateFailedIsTracked(t *testing.T) {
	p, sum := blacklistFile(t, "pass")
	api := &blacklistAPI{statuses: []string{"failed"}, sha: sum}
	srv := api.server(t)
	defer srv.Close()

	plan := unknownComputed(blacklistModel(p, "common", ""))
	plan.ApplyNow = types.BoolValue(true)
	plan.ContentSHA256 = types.StringValue(sum)
	got, resp := runBlacklistCreate(t, srv.URL, plan)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a failed file")
	}
	if got.ID.ValueString() != "common" || got.Status.ValueString() != "failed" {
		t.Errorf("a failed file must stay in state so it can be deleted: %+v", got)
	}
}

func TestPasswordBlacklistFileReadRemovesMissing(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	s := blacklistSchema(t)
	r := &passwordBlacklistFileResource{client: skycloak.New(srv.URL, "sk_sc_test_aaa_bbb", "")}
	raw := blacklistRaw(t, s, blacklistModel("x", "common", "abc"))
	resp := resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: raw}}, &resp)
	if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
		t.Fatalf("a missing file must leave state: %v", resp.Diagnostics)
	}
}

func TestPasswordBlacklistFileDelete(t *testing.T) {
	api := &blacklistAPI{statuses: []string{"ready"}}
	srv := api.server(t)
	defer srv.Close()
	s := blacklistSchema(t)
	r := &passwordBlacklistFileResource{client: skycloak.New(srv.URL, "sk_sc_test_aaa_bbb", "")}
	raw := blacklistRaw(t, s, blacklistModel("x", "common", "abc"))
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: raw}}, &resp)
	if resp.Diagnostics.HasError() || api.delete != 1 {
		t.Fatalf("deletes = %d, diagnostics: %v", api.delete, resp.Diagnostics)
	}
}

func TestPasswordBlacklistFileDeleteRefusedWhilePolicyNamesIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"title":"Conflict","status":409}`))
	}))
	defer srv.Close()
	s := blacklistSchema(t)
	r := &passwordBlacklistFileResource{client: skycloak.New(srv.URL, "sk_sc_test_aaa_bbb", "")}
	raw := blacklistRaw(t, s, blacklistModel("x", "common", "abc"))
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: raw}}
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: raw}}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused delete must fail the apply")
	}
}

func TestPasswordBlacklistFileImportState(t *testing.T) {
	s := blacklistSchema(t)
	r := &passwordBlacklistFileResource{}
	for _, tt := range []struct {
		id      string
		wantErr bool
	}{
		{testBlacklistClusterID + "/common.txt", false},
		{"common.txt", true},
		{"/common.txt", true},
		{testBlacklistClusterID + "/", true},
	} {
		resp := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}}
		r.ImportState(context.Background(), resource.ImportStateRequest{ID: tt.id}, &resp)
		if resp.Diagnostics.HasError() != tt.wantErr {
			t.Errorf("import %q: error = %v, want %v", tt.id, resp.Diagnostics.HasError(), tt.wantErr)
		}
		if !tt.wantErr {
			var got passwordBlacklistFileResourceModel
			resp.State.Get(context.Background(), &got)
			if got.ClusterID.ValueString() != testBlacklistClusterID || got.Name.ValueString() != "common.txt" {
				t.Errorf("imported %+v", got)
			}
		}
	}
}
