package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverOIDC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity-providers/discover-oidc" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, `{"issuer":"https://accounts.google.com","authorization_endpoint":"https://accounts.google.com/o/oauth2/v2/auth","token_endpoint":"https://oauth2.googleapis.com/token","userinfo_endpoint":"https://openidconnect.googleapis.com/v1/userinfo","jwks_uri":"https://www.googleapis.com/oauth2/v3/certs","scopes_supported":["openid","email","profile"]}`)
	}))
	defer srv.Close()

	doc, err := newTestClient(srv.URL).DiscoverOIDC(context.Background(), "https://accounts.google.com")
	if err != nil || doc.TokenEndpoint == "" || len(doc.ScopesSupported) != 3 {
		t.Fatalf("DiscoverOIDC: %+v, %v", doc, err)
	}
}
