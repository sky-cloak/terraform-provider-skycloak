package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadMetadataEndpoints(t *testing.T) {
	const dom = "22222222-2222-2222-2222-222222222222"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cluster-types/keycloak/versions":
			writeJSON(w, 200, `["26.1","26.0","25.0"]`)
		case "/identity-provider-templates":
			writeJSON(w, 200, `[{"id":"google","name":"Google","description":"Google SSO","type":"oidc"}]`)
		case "/clusters/" + cuid + "/domains/" + dom + "/routes":
			writeJSON(w, 200, `[{"id":"33333333-3333-3333-3333-333333333333","cluster_id":"`+cuid+`","domain_id":"`+dom+`","realm":"app","allow_admin_access":false,"hide_realm_path":true,"cors_allowed_origins":[],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`)
		case "/clusters/" + cuid + "/builds":
			writeJSON(w, 200, `[{"id":"`+dom+`","status":"completed","phase":"done","progress":100,"error":null,"started_at":"2026-01-01T00:00:00Z","completed_at":"2026-01-01T00:05:00Z"}]`)
		case "/clusters/" + cuid + "/upgrades":
			writeJSON(w, 200, `[{"id":"up1","from_version":"25.0","to_version":"26.1","phase":"completed","started_at":"2026-01-01T00:00:00Z","completed_at":"2026-01-01T00:30:00Z"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if vs, err := c.ClusterTypeVersions(context.Background(), "keycloak"); err != nil || len(vs) != 3 {
		t.Fatalf("ClusterTypeVersions: %+v, %v", vs, err)
	}
	if ts, err := c.ListIdentityProviderTemplates(context.Background()); err != nil || ts[0].Type != "oidc" {
		t.Fatalf("ListIdentityProviderTemplates: %+v, %v", ts, err)
	}
	if rs, err := c.ListDomainRoutes(context.Background(), cuid, dom); err != nil || len(rs) != 1 || !rs[0].HideRealmPath {
		t.Fatalf("ListDomainRoutes: %+v, %v", rs, err)
	}
	if us, err := c.ListClusterUpgrades(context.Background(), cuid); err != nil || us[0].ToVersion != "26.1" {
		t.Fatalf("ListClusterUpgrades: %+v, %v", us, err)
	}
}
