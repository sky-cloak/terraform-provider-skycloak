package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndGetCluster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") == "" {
			t.Error("missing apikey header")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/clusters":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"c1","name":"prod","type":"keycloak","size":"small","version":"26.1","location":"eu","status":"provisioning"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/clusters/c1":
			_, _ = w.Write([]byte(`{"id":"c1","name":"prod","status":"available"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "sk_sc_test_a_b", "")

	created, err := c.CreateCluster(context.Background(), CreateClusterRequest{
		Name: "prod", Type: "keycloak", Size: "small", Version: "26.1", Location: "eu",
	})
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	if created.ID != "c1" || created.Status != "provisioning" {
		t.Fatalf("unexpected created cluster: %+v", created)
	}

	got, err := c.GetCluster(context.Background(), "c1")
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	if got.Status != "available" {
		t.Fatalf("status = %q, want available", got.Status)
	}
}

func TestRealmCRUD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/clusters/c1/realms":
			_, _ = w.Write([]byte(`{"name":"acme","display_name":"Acme","enabled":true,"ssl_required":"external"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/clusters/c1/realms/acme":
			_, _ = w.Write([]byte(`{"name":"acme","display_name":"Acme","enabled":true,"ssl_required":"external"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/clusters/c1/realms/acme":
			_, _ = w.Write([]byte(`{"name":"acme","display_name":"Acme Updated","enabled":false,"ssl_required":"all"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/clusters/c1/realms/acme":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "")
	created, err := c.CreateRealm(context.Background(), "c1", Realm{Name: "acme", DisplayName: "Acme", Enabled: true, SSLRequired: "external"})
	if err != nil || created.Name != "acme" {
		t.Fatalf("CreateRealm: %+v, %v", created, err)
	}
	got, err := c.GetRealm(context.Background(), "c1", "acme")
	if err != nil || !got.Enabled {
		t.Fatalf("GetRealm: %+v, %v", got, err)
	}
	upd, err := c.UpdateRealm(context.Background(), "c1", "acme", Realm{Name: "acme", DisplayName: "Acme Updated", SSLRequired: "all"})
	if err != nil || upd.DisplayName != "Acme Updated" || upd.Enabled {
		t.Fatalf("UpdateRealm: %+v, %v", upd, err)
	}
	if err := c.DeleteRealm(context.Background(), "c1", "acme"); err != nil {
		t.Fatalf("DeleteRealm: %v", err)
	}
}

func TestApplicationCRUD(t *testing.T) {
	base := "/clusters/c1/realms/app/applications"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			_, _ = w.Write([]byte(`{"client_id":"web","type":"confidential","protocol":"openid-connect","client_secret":"s3cr3t","redirect_uris":["https://app/cb"]}`))
		case r.Method == http.MethodGet && r.URL.Path == base+"/web":
			_, _ = w.Write([]byte(`{"client_id":"web","type":"confidential","protocol":"openid-connect","redirect_uris":["https://app/cb"]}`))
		case r.Method == http.MethodDelete && r.URL.Path == base+"/web":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "")
	created, err := c.CreateApplication(context.Background(), "c1", "app", Application{ClientID: "web", Type: "confidential", RedirectURIs: []string{"https://app/cb"}})
	if err != nil || created.ClientSecret != "s3cr3t" {
		t.Fatalf("CreateApplication: %+v, %v", created, err)
	}
	got, err := c.GetApplication(context.Background(), "c1", "app", "web")
	if err != nil || len(got.RedirectURIs) != 1 {
		t.Fatalf("GetApplication: %+v, %v", got, err)
	}
	if err := c.DeleteApplication(context.Background(), "c1", "app", "web"); err != nil {
		t.Fatalf("DeleteApplication: %v", err)
	}
}

func TestIdentityProviderCRUD(t *testing.T) {
	base := "/clusters/c1/realms/app/identity-providers"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			_, _ = w.Write([]byte(`{"provider_id":"google","type":"oidc","enabled":true,"config":{"clientId":"abc"}}`))
		case r.Method == http.MethodGet && r.URL.Path == base+"/google":
			_, _ = w.Write([]byte(`{"provider_id":"google","type":"oidc","enabled":true,"config":{"clientId":"abc"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == base+"/google":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "")
	created, err := c.CreateIdentityProvider(context.Background(), "c1", "app", IdentityProvider{ProviderID: "google", Type: "oidc", Enabled: true, Config: map[string]string{"clientId": "abc"}})
	if err != nil || created.Config["clientId"] != "abc" {
		t.Fatalf("CreateIdentityProvider: %+v, %v", created, err)
	}
	got, err := c.GetIdentityProvider(context.Background(), "c1", "app", "google")
	if err != nil || !got.Enabled {
		t.Fatalf("GetIdentityProvider: %+v, %v", got, err)
	}
	if err := c.DeleteIdentityProvider(context.Background(), "c1", "app", "google"); err != nil {
		t.Fatalf("DeleteIdentityProvider: %v", err)
	}
}

func TestSMTPUpsertGetDelete(t *testing.T) {
	p := "/clusters/c1/realms/app/smtp"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			_, _ = w.Write([]byte(`{"host":"smtp.example.com","port":587,"encryption":"starttls","from_email":"no-reply@example.com","auth_type":"basic","username":"u","has_password":true,"status":"configured"}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"host":"smtp.example.com","port":587,"encryption":"starttls","from_email":"no-reply@example.com","auth_type":"basic","username":"u","has_password":true,"status":"configured"}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
		_ = p
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "")
	cfg, err := c.UpsertSMTP(context.Background(), "c1", "app", UpsertSMTPRequest{Host: "smtp.example.com", Port: 587, Encryption: "starttls", FromEmail: "no-reply@example.com", AuthType: "basic", Username: "u", Password: "secret"})
	if err != nil || cfg.Port != 587 || !cfg.HasPassword {
		t.Fatalf("UpsertSMTP: %+v, %v", cfg, err)
	}
	got, err := c.GetSMTP(context.Background(), "c1", "app")
	if err != nil || got.Status != "configured" {
		t.Fatalf("GetSMTP: %+v, %v", got, err)
	}
	if err := c.DeleteSMTP(context.Background(), "c1", "app"); err != nil {
		t.Fatalf("DeleteSMTP: %v", err)
	}
}

func TestListClusterMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cluster-locations":
			_, _ = w.Write([]byte(`[{"location":"eu","name":"Europe","available":true}]`))
		case "/cluster-types":
			_, _ = w.Write([]byte(`[{"type":"keycloak","name":"Keycloak","available":true}]`))
		case "/cluster-features":
			_, _ = w.Write([]byte(`[{"name":"token-exchange","display_name":"Token Exchange","description":null,"preview":true,"min_version":"26.0","max_version":null}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "")
	locs, err := c.ListClusterLocations(context.Background())
	if err != nil || len(locs) != 1 || locs[0].Location != "eu" {
		t.Fatalf("ListClusterLocations: %+v, %v", locs, err)
	}
	tys, err := c.ListClusterTypes(context.Background())
	if err != nil || tys[0].Type != "keycloak" {
		t.Fatalf("ListClusterTypes: %+v, %v", tys, err)
	}
	feats, err := c.ListClusterFeatures(context.Background())
	if err != nil || feats[0].Description != nil || feats[0].MaxVersion != nil || feats[0].MinVersion == nil {
		t.Fatalf("ListClusterFeatures nullable handling: %+v, %v", feats, err)
	}
}

func TestIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"title":"Not Found","status":404}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "key", "")
	_, err := c.GetCluster(context.Background(), "missing")
	if !IsNotFound(err) {
		t.Fatalf("want IsNotFound, got %v", err)
	}
}
