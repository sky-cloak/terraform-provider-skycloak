package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const extUID = "55555555-5555-5555-5555-555555555555"

func TestExtensionCatalog(t *testing.T) {
	catalog := `[{"id":"` + extUID + `","name":"Magic Link","description":"Passwordless login","source":"marketplace",` +
		`"keycloak_versions":["25","26"],"documentation_url":"https://docs.example.com","repository_url":null,"icon_url":null,` +
		`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","parameter_type":"none","parameters":[],"previous_versions":[],` +
		`"file_size_bytes":null,"original_jar_name":null,"quick_start_steps":null,"scan_message":null,"scan_status":null}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/extensions":
			writeJSON(w, 200, catalog)
		case "/extensions/" + extUID:
			writeJSON(w, 200, catalog[1:len(catalog)-1])
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	list, err := c.ListExtensions(context.Background())
	if err != nil || len(list) != 1 || list[0].Name != "Magic Link" || len(list[0].KeycloakVersions) != 2 {
		t.Fatalf("ListExtensions: %+v, %v", list, err)
	}
	if list[0].RepositoryURL != "" {
		t.Fatalf("expected null repository_url to map to empty, got %q", list[0].RepositoryURL)
	}
	got, err := c.GetExtension(context.Background(), extUID)
	if err != nil || got.Description != "Passwordless login" {
		t.Fatalf("GetExtension: %+v, %v", got, err)
	}
}

func TestClusterExtensionLifecycle(t *testing.T) {
	installed := `{"extension_id":"` + extUID + `","extension_name":"Magic Link","extension_source":"marketplace",` +
		`"installed_at":"2026-01-01T00:00:00Z","installed_version":"1.0.0","available_version":"1.1.0",` +
		`"last_status_change_at":null,"parameters":{},"status":"installing","upgrade_available":true}`
	base := "/clusters/" + cuid + "/extensions"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == base:
			writeJSON(w, 200, "["+installed+"]")
		case r.Method == http.MethodPost && r.URL.Path == base:
			writeJSON(w, http.StatusAccepted, installed)
		case r.Method == http.MethodPost && r.URL.Path == base+"/"+extUID+"/upgrade":
			writeJSON(w, http.StatusAccepted, installed)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/"+extUID:
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	ext, err := c.InstallExtension(context.Background(), cuid, extUID, map[string]string{"api_key": "secret"})
	if err != nil || ext.Status != "installing" || !ext.UpgradeAvailable || ext.AvailableVersion != "1.1.0" {
		t.Fatalf("InstallExtension: %+v, %v", ext, err)
	}
	got, err := c.GetClusterExtension(context.Background(), cuid, extUID)
	if err != nil || got.InstalledVersion != "1.0.0" {
		t.Fatalf("GetClusterExtension: %+v, %v", got, err)
	}
	if _, err := c.UpgradeClusterExtension(context.Background(), cuid, extUID); err != nil {
		t.Fatalf("UpgradeClusterExtension: %v", err)
	}
	if err := c.UninstallExtension(context.Background(), cuid, extUID); err != nil {
		t.Fatalf("UninstallExtension: %v", err)
	}
}

func TestGetClusterExtensionNotInstalled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, "[]")
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).GetClusterExtension(context.Background(), cuid, extUID)
	if !IsNotFound(err) {
		t.Fatalf("want IsNotFound for an uninstalled extension, got %v", err)
	}
}
