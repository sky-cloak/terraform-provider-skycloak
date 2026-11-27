package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/platformclient"
)

func TestNewPlatformSendsBearerKeyAndUserAgent(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewPlatform(srv.URL, "pk_test", WithUserAgent("terraform-provider-skycloak/test"))
	if _, err := c.API().SessionsReadWithResponse(context.Background()); err != nil {
		t.Fatal(err)
	}

	if v := got.Get("Authorization"); v != "Bearer pk_test" {
		t.Errorf("Authorization = %q, want the key as a bearer token", v)
	}
	if v := got.Get("User-Agent"); v != "terraform-provider-skycloak/test" {
		t.Errorf("User-Agent = %q", v)
	}
	if v := got.Get("API-Key"); v != "" {
		t.Errorf("API-Key header is for the managed Keycloak API, got %q", v)
	}
}

func TestNewPlatformRetriesTransientStatus(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewPlatform(srv.URL, "pk_test")
	resp, err := c.API().SessionsReadWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || resp.StatusCode() != http.StatusNoContent {
		t.Errorf("calls = %d, status = %d; want one retry then 204", calls, resp.StatusCode())
	}
}

func TestNewPlatformDefaultEndpoint(t *testing.T) {
	c := NewPlatform("", "pk_test")
	if got := c.API().ClientInterface.(*platformclient.Client).Server; got != DefaultPlatformEndpoint+"/" {
		t.Errorf("server = %q, want %q", got, DefaultPlatformEndpoint+"/")
	}
}
