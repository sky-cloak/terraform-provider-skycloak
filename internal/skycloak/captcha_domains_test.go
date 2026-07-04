package skycloak

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCAPTCHADomainLifecycle(t *testing.T) {
	base := "/clusters/" + cuid + "/security/captcha/domains"
	var addBody string
	var removed string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == base:
			writeJSON(w, 200, `{"domains":[{"hostname":"login.example.com","created_at":"2026-07-01T00:00:00Z"}],"max_allowed":5}`)
		case r.Method == http.MethodPost && r.URL.Path == base:
			raw, _ := io.ReadAll(r.Body)
			addBody = string(raw)
			writeJSON(w, 201, `{"hostname":"id.example.com","created_at":"2026-07-04T00:00:00Z"}`)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/id.example.com":
			removed = "id.example.com"
			w.WriteHeader(204)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	list, err := c.ListCAPTCHADomains(context.Background(), cuid)
	if err != nil {
		t.Fatalf("ListCAPTCHADomains: %v", err)
	}
	if list.MaxAllowed != 5 || len(list.Domains) != 1 || list.Domains[0].Hostname != "login.example.com" {
		t.Fatalf("unexpected list: %+v", list)
	}

	added, err := c.AddCAPTCHADomain(context.Background(), cuid, "id.example.com")
	if err != nil {
		t.Fatalf("AddCAPTCHADomain: %v", err)
	}
	if added.Hostname != "id.example.com" || added.CreatedAt == "" {
		t.Fatalf("unexpected add result: %+v", added)
	}
	var sent map[string]string
	if err := json.Unmarshal([]byte(addBody), &sent); err != nil || sent["hostname"] != "id.example.com" {
		t.Fatalf("unexpected add body: %s (%v)", addBody, err)
	}

	if err := c.RemoveCAPTCHADomain(context.Background(), cuid, "id.example.com"); err != nil {
		t.Fatalf("RemoveCAPTCHADomain: %v", err)
	}
	if removed != "id.example.com" {
		t.Fatal("remove endpoint not called with hostname")
	}
}

func TestAddCAPTCHADomainLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 422, `{"title":"limit reached","detail":"max domains registered","status":422}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).AddCAPTCHADomain(context.Background(), cuid, "x.example.com")
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.StatusCode != 422 {
		t.Fatalf("want 422 APIError, got %v", err)
	}
}
