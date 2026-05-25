package skycloak

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cuid is a valid UUID used as the cluster ID in tests (cluster IDs are UUIDs).
const cuid = "11111111-1111-1111-1111-111111111111"

func newTestClient(url string) *Client {
	return New(url, "sk_sc_test_aaa_bbb", "2026-03-01")
}

// writeJSON writes a JSON body with the Content-Type the generated client
// requires in order to unmarshal a typed response.
func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func TestListClusters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("apikey"); got != "sk_sc_test_aaa_bbb" {
			t.Errorf("apikey header = %q", got)
		}
		if got := r.Header.Get("API-Version"); got != "2026-03-01" {
			t.Errorf("API-Version header = %q", got)
		}
		if r.URL.Path != "/clusters" {
			t.Errorf("path = %q", r.URL.Path)
		}
		writeJSON(w, 200, `[{"id":"`+cuid+`","name":"prod","status":"available"}]`)
	}))
	defer srv.Close()

	clusters, err := newTestClient(srv.URL).ListClusters(context.Background())
	if err != nil {
		t.Fatalf("ListClusters: %v", err)
	}
	if len(clusters) != 1 || clusters[0].Name != "prod" {
		t.Fatalf("unexpected clusters: %+v", clusters)
	}
}

func TestCreateAndGetCluster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/clusters":
			writeJSON(w, http.StatusCreated, `{"id":"`+cuid+`","name":"prod","type":"keycloak","size":"small","version":"26.1","location":"eu","status":"provisioning"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/clusters/"+cuid:
			writeJSON(w, 200, `{"id":"`+cuid+`","name":"prod","status":"available"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	created, err := c.CreateCluster(context.Background(), CreateClusterRequest{Name: "prod", Type: "keycloak", Size: "small", Version: "26.1", Location: "eu"})
	if err != nil || created.ID != cuid || created.Status != "provisioning" {
		t.Fatalf("CreateCluster: %+v, %v", created, err)
	}
	got, err := c.GetCluster(context.Background(), cuid)
	if err != nil || got.Status != "available" {
		t.Fatalf("GetCluster: %+v, %v", got, err)
	}
}

func TestRealmCRUD(t *testing.T) {
	base := "/clusters/" + cuid + "/realms"
	body := `{"name":"acme","display_name":"Acme","enabled":true,"ssl_required":"external"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			writeJSON(w, http.StatusCreated, body)
		case r.Method == http.MethodGet && r.URL.Path == base+"/acme":
			writeJSON(w, 200, body)
		case r.Method == http.MethodPatch && r.URL.Path == base+"/acme":
			writeJSON(w, 200, body)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/acme":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	created, err := c.CreateRealm(context.Background(), cuid, Realm{Name: "acme", DisplayName: "Acme", Enabled: true, SSLRequired: "external"})
	if err != nil || created.Name != "acme" {
		t.Fatalf("CreateRealm: %+v, %v", created, err)
	}
	if _, err := c.GetRealm(context.Background(), cuid, "acme"); err != nil {
		t.Fatalf("GetRealm: %v", err)
	}
	if err := c.DeleteRealm(context.Background(), cuid, "acme"); err != nil {
		t.Fatalf("DeleteRealm: %v", err)
	}
}

func TestApplicationCRUD(t *testing.T) {
	base := "/clusters/" + cuid + "/realms/app/applications"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			writeJSON(w, http.StatusCreated, `{"client_id":"web","name":"Web","type":"confidential","protocol":"openid-connect","status":"active","client_secret":"s3cr3t","redirect_uris":["https://app/cb"],"grant_types":[]}`)
		case r.Method == http.MethodGet && r.URL.Path == base+"/web":
			writeJSON(w, 200, `{"client_id":"web","name":"Web","type":"confidential","protocol":"openid-connect","status":"active","redirect_uris":["https://app/cb"],"grant_types":[]}`)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/web":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	created, err := c.CreateApplication(context.Background(), cuid, "app", Application{ClientID: "web", Name: "Web", Type: "confidential", RedirectURIs: []string{"https://app/cb"}})
	if err != nil || created.ClientSecret != "s3cr3t" {
		t.Fatalf("CreateApplication: %+v, %v", created, err)
	}
	got, err := c.GetApplication(context.Background(), cuid, "app", "web")
	if err != nil || len(got.RedirectURIs) != 1 {
		t.Fatalf("GetApplication: %+v, %v", got, err)
	}
	if err := c.DeleteApplication(context.Background(), cuid, "app", "web"); err != nil {
		t.Fatalf("DeleteApplication: %v", err)
	}
}

func TestSMTPUpsertGetDelete(t *testing.T) {
	body := `{"host":"smtp.example.com","port":587,"encryption":"starttls","from_email":"no-reply@example.com","auth_type":"basic","username":"u","has_password":true,"has_client_secret":false,"status":"configured","cluster_id":"` + cuid + `","realm":"app","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut, http.MethodGet:
			writeJSON(w, 200, body)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	cfg, err := c.UpsertSMTP(context.Background(), cuid, "app", UpsertSMTPRequest{Host: "smtp.example.com", Port: 587, Encryption: "starttls", FromEmail: "no-reply@example.com", AuthType: "basic", Username: "u", Password: "secret"})
	if err != nil || cfg.Port != 587 || !cfg.HasPassword {
		t.Fatalf("UpsertSMTP: %+v, %v", cfg, err)
	}
	got, err := c.GetSMTP(context.Background(), cuid, "app")
	if err != nil || got.Status != "configured" {
		t.Fatalf("GetSMTP: %+v, %v", got, err)
	}
	if err := c.DeleteSMTP(context.Background(), cuid, "app"); err != nil {
		t.Fatalf("DeleteSMTP: %v", err)
	}
}

func TestListClusterMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cluster-locations":
			writeJSON(w, 200, `[{"location":"eu","name":"Europe","available":true}]`)
		case "/cluster-types":
			writeJSON(w, 200, `[{"type":"keycloak","name":"Keycloak","available":true}]`)
		case "/cluster-features":
			writeJSON(w, 200, `[{"name":"token-exchange","display_name":"Token Exchange","description":null,"preview":true,"min_version":"26.0","max_version":null}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
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

func TestRotateApplicationSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/clusters/"+cuid+"/realms/app/applications/web/rotate-secret" {
			writeJSON(w, 200, `{"client_secret":"rotated-secret"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	secret, err := newTestClient(srv.URL).RotateApplicationSecret(context.Background(), cuid, "app", "web")
	if err != nil || secret != "rotated-secret" {
		t.Fatalf("RotateApplicationSecret: %q, %v", secret, err)
	}
}

func TestListApplicationsPaginates(t *testing.T) {
	base := "/clusters/" + cuid + "/realms/app/applications"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First page returns a full page (100) → triggers a second request.
		if r.URL.Query().Get("offset") == "0" {
			var b strings.Builder
			b.WriteString("[")
			for i := 0; i < 100; i++ {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"client_id":"c%d","name":"n","type":"confidential","protocol":"openid-connect","status":"active","grant_types":[]}`, i)
			}
			b.WriteString("]")
			writeJSON(w, 200, b.String())
			return
		}
		writeJSON(w, 200, `[{"client_id":"last","name":"n","type":"confidential","protocol":"openid-connect","status":"active","grant_types":[]}]`)
		_ = base
	}))
	defer srv.Close()

	apps, err := newTestClient(srv.URL).ListApplications(context.Background(), cuid, "app")
	if err != nil || len(apps) != 101 {
		t.Fatalf("ListApplications pagination: got %d apps, err %v", len(apps), err)
	}
}

func TestIdentityProviderCRUD(t *testing.T) {
	base := "/clusters/" + cuid + "/realms/app/identity-providers"
	body := `{"provider_id":"google","type":"oidc","display_name":"Google","enabled":true,"externally_managed":false,"config":{"button_text":"Sign in with Google","oidc":{"issuer":"https://accounts.google.com","token_url":"https://oauth2.googleapis.com/token"},"attribute_mappings":{"email":"email"}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			writeJSON(w, http.StatusCreated, body)
		case r.Method == http.MethodPatch && r.URL.Path == base+"/google":
			writeJSON(w, 200, body)
		case r.Method == http.MethodGet && r.URL.Path == base+"/google":
			writeJSON(w, 200, body)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/google":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	created, err := c.CreateIdentityProvider(context.Background(), cuid, "app", IdentityProvider{
		ProviderID: "google", Type: "oidc", DisplayName: "Google", Enabled: true,
		Config: ProviderConfig{ButtonText: "Sign in with Google", OIDC: &OIDCConfig{Issuer: "https://accounts.google.com", TokenURL: "https://oauth2.googleapis.com/token"}},
	})
	if err != nil || created.Config.OIDC == nil || created.Config.OIDC.Issuer != "https://accounts.google.com" {
		t.Fatalf("CreateIdentityProvider: %+v, %v", created, err)
	}
	got, err := c.GetIdentityProvider(context.Background(), cuid, "app", "google")
	if err != nil || got.Config.AttributeMappings["email"] != "email" {
		t.Fatalf("GetIdentityProvider: %+v, %v", got, err)
	}
	if err := c.DeleteIdentityProvider(context.Background(), cuid, "app", "google"); err != nil {
		t.Fatalf("DeleteIdentityProvider: %v", err)
	}
}

func TestProblemError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"title":"Forbidden","status":403,"detail":"missing scope clusters:read"}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).ListClusters(context.Background())
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("want *APIError, got %T (%v)", err, err)
	}
	if apiErr.StatusCode != 403 || apiErr.Problem.Detail == "" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
}

func TestIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"title":"Not Found","status":404}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).GetCluster(context.Background(), cuid)
	if !IsNotFound(err) {
		t.Fatalf("want IsNotFound, got %v", err)
	}
}
