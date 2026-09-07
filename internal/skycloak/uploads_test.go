package skycloak

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// parseMultipart reads a request's multipart body into field→value (text) and
// a set of file part field names with their declared Content-Type.
func parseMultipart(t *testing.T, r *http.Request) (map[string][]string, map[string]string) {
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
		if p.FileName() != "" || p.Header.Get("Content-Type") != "" {
			fileCT[p.FormName()] = p.Header.Get("Content-Type")
		}
		fields[p.FormName()] = append(fields[p.FormName()], string(body))
	}
	return fields, fileCT
}

func TestUploadThemeMultipart(t *testing.T) {
	var sawName, sawFileCT, sawThemeType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields, fileCT := parseMultipart(t, r)
		sawName = strings.Join(fields["name"], "")
		sawFileCT = fileCT["theme_file"]
		sawThemeType = strings.Join(fields["theme_types"], ",")
		writeJSON(w, http.StatusCreated, `{"id":"`+themeUID+`","cluster_id":"`+cuid+`","name":"corp","status":"deploying","theme_types":["login"],"file_size":12,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	theme, err := c.UploadTheme(context.Background(), cuid, UploadThemeRequest{
		Name: "corp", Version: "1.0.0", ThemeTypes: []string{"login"}, FileName: "corp.zip", Content: []byte("PK\x03\x04zip"),
	})
	if err != nil || theme.ID != themeUID {
		t.Fatalf("UploadTheme: %+v, %v", theme, err)
	}
	if sawName != "corp" {
		t.Fatalf("name field = %q", sawName)
	}
	if sawFileCT != "application/zip" {
		t.Fatalf("theme_file content-type = %q, want application/zip", sawFileCT)
	}
	if sawThemeType != "login" {
		t.Fatalf("theme_types = %q", sawThemeType)
	}
}

func TestUploadThemeJARContentType(t *testing.T) {
	var sawCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, fileCT := parseMultipart(t, r)
		sawCT = fileCT["theme_file"]
		writeJSON(w, http.StatusCreated, `{"id":"`+themeUID+`","cluster_id":"`+cuid+`","name":"k","status":"deploying","theme_types":[],"file_size":1,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`)
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).UploadTheme(context.Background(), cuid, UploadThemeRequest{Name: "k", FileName: "theme.jar", Content: []byte("CAFEBABE")}); err != nil {
		t.Fatalf("UploadTheme jar: %v", err)
	}
	if sawCT != "application/java-archive" {
		t.Fatalf("jar content-type = %q", sawCT)
	}
}

func TestThemeMetadataUpdateAndDelete(t *testing.T) {
	base := "/clusters/" + cuid + "/themes/" + themeUID
	var sawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch, http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			sawBody = string(b)
			writeJSON(w, 200, `{"id":"`+themeUID+`","cluster_id":"`+cuid+`","name":"renamed","status":"deployed","theme_types":[],"file_size":1,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
		_ = base
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.UpdateThemeMetadata(context.Background(), cuid, themeUID, "renamed", "", "")
	if err != nil || got.Name != "renamed" {
		t.Fatalf("UpdateThemeMetadata: %+v, %v", got, err)
	}
	// An empty version must reach the API as an explicit empty string: omitting
	// it would keep the stored label, so a removal could never be expressed.
	if !strings.Contains(sawBody, `"version":""`) {
		t.Errorf("metadata body = %s, want an explicit empty version", sawBody)
	}
	if err := c.DeleteTheme(context.Background(), cuid, themeUID); err != nil {
		t.Fatalf("DeleteTheme: %v", err)
	}
}

// TestUpdateThemeContentMultipart covers replacing a theme's archive in place:
// the request must be a PUT to the theme's /content sub-resource carrying only
// the new archive (and an optional version), and the theme it returns keeps its
// identity.
func TestUpdateThemeContentMultipart(t *testing.T) {
	var (
		sawMethod, sawPath, sawFileCT, sawVersion, sawBody string
		sawFields                                          []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawMethod, sawPath = r.Method, r.URL.Path
		fields, fileCT := parseMultipart(t, r)
		sawFileCT = fileCT["theme_file"]
		sawVersion = strings.Join(fields["version"], "")
		sawBody = strings.Join(fields["theme_file"], "")
		for name := range fields {
			sawFields = append(sawFields, name)
		}
		writeJSON(w, 200, `{"id":"`+themeUID+`","cluster_id":"`+cuid+`","name":"corp","version":"1.1.0","status":"deploying","theme_types":["login"],"file_size":9,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}`)
	}))
	defer srv.Close()

	theme, err := newTestClient(srv.URL).UpdateThemeContent(context.Background(), cuid, themeUID, UpdateThemeContentRequest{
		Version: "1.1.0", FileName: "corp-v2.zip", Content: []byte("PK\x03\x04new"),
	})
	if err != nil {
		t.Fatalf("UpdateThemeContent: %v", err)
	}
	if theme.ID != themeUID || theme.Name != "corp" || theme.Status != "deploying" {
		t.Fatalf("theme = %+v, want the same id and name back", theme)
	}
	if sawMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", sawMethod)
	}
	if want := "/clusters/" + cuid + "/themes/" + themeUID + "/content"; sawPath != want {
		t.Errorf("path = %q, want %q", sawPath, want)
	}
	if sawFileCT != "application/zip" {
		t.Errorf("theme_file content-type = %q, want application/zip", sawFileCT)
	}
	if sawBody != "PK\x03\x04new" {
		t.Errorf("theme_file body = %q, want the new archive bytes", sawBody)
	}
	if sawVersion != "1.1.0" {
		t.Errorf("version field = %q, want 1.1.0", sawVersion)
	}
	// name and theme_types are not part of the content body: the endpoint keeps
	// the theme's identity and its deployed types.
	for _, f := range sawFields {
		if f != "theme_file" && f != "version" {
			t.Errorf("unexpected form field %q in the content update", f)
		}
	}
}

// TestUpdateThemeContentJARAndConflict covers the JAR content type and the
// documented 409 (a migration-created theme whose content is pinned), which
// must surface as an API error rather than a nil theme.
func TestUpdateThemeContentJARAndConflict(t *testing.T) {
	var sawCT string
	conflict := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conflict {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"title":"Conflict","detail":"theme content is pinned"}`))
			return
		}
		_, fileCT := parseMultipart(t, r)
		sawCT = fileCT["theme_file"]
		writeJSON(w, 200, `{"id":"`+themeUID+`","cluster_id":"`+cuid+`","name":"k","status":"deploying","theme_types":[],"file_size":8,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.UpdateThemeContent(context.Background(), cuid, themeUID, UpdateThemeContentRequest{FileName: "theme.jar", Content: []byte("CAFEBABE")}); err != nil {
		t.Fatalf("UpdateThemeContent jar: %v", err)
	}
	if sawCT != "application/java-archive" {
		t.Errorf("jar content-type = %q", sawCT)
	}

	conflict = true
	theme, err := c.UpdateThemeContent(context.Background(), cuid, themeUID, UpdateThemeContentRequest{FileName: "theme.zip", Content: []byte("PK")})
	if err == nil || theme != nil {
		t.Fatalf("UpdateThemeContent on 409 = %+v, %v; want an error", theme, err)
	}
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("error = %v, want an APIError with status 409", err)
	}
}

func TestUploadExtensionMultipart(t *testing.T) {
	var jarCT, metaCT, meta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields, fileCT := parseMultipart(t, r)
		jarCT = fileCT["jar"]
		metaCT = fileCT["metadata"]
		meta = strings.Join(fields["metadata"], "")
		writeJSON(w, http.StatusCreated, `{"id":"`+extUID+`","name":"Magic Link","description":null,"documentation_url":null,"repository_url":null,"icon_url":null,"keycloak_versions":["26"],"source":"custom","parameter_type":"none","parameters":[],"previous_versions":[],"file_size_bytes":10,"original_jar_name":"ml.jar","quick_start_steps":null,"scan_message":null,"scan_status":"clean","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	ext, err := c.UploadExtension(context.Background(), UploadExtensionRequest{
		Name: "Magic Link", KeycloakVersion: "26", Version: "1.0.0", JarFileName: "ml.jar", Jar: []byte("CAFEBABE"),
		Parameters: []ExtensionParameterDef{{Key: "sender", Label: "Sender", Type: "text", Required: true}},
	})
	if err != nil || ext.ID != extUID || ext.ScanStatus != "clean" {
		t.Fatalf("UploadExtension: %+v, %v", ext, err)
	}
	if jarCT != "application/java-archive" {
		t.Fatalf("jar content-type = %q", jarCT)
	}
	if metaCT != "application/json" {
		t.Fatalf("metadata content-type = %q", metaCT)
	}
	if !strings.Contains(meta, `"keycloak_version":"26"`) || !strings.Contains(meta, `"sender"`) {
		t.Fatalf("metadata JSON missing fields: %s", meta)
	}
}

func TestPublishExtensionVersionAndDelete(t *testing.T) {
	base := "/extensions/" + extUID
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/versions"):
			_, fileCT := parseMultipart(t, r)
			if fileCT["jar"] != "application/java-archive" {
				t.Errorf("publish jar content-type = %q", fileCT["jar"])
			}
			writeJSON(w, 200, `{"id":"`+extUID+`","name":"Magic Link","description":null,"documentation_url":null,"repository_url":null,"icon_url":null,"keycloak_versions":["26"],"source":"custom","parameter_type":"none","parameters":[],"previous_versions":[],"file_size_bytes":10,"original_jar_name":"ml.jar","quick_start_steps":null,"scan_message":null,"scan_status":"clean","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`)
		case r.Method == http.MethodDelete && r.URL.Path == base:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.PublishExtensionVersion(context.Background(), extUID, "1.1.0", "ml.jar", []byte("CAFEBABE")); err != nil {
		t.Fatalf("PublishExtensionVersion: %v", err)
	}
	if err := c.DeleteExtension(context.Background(), extUID); err != nil {
		t.Fatalf("DeleteExtension: %v", err)
	}
}
