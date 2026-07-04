package skycloak

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTestSMTP(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/realms/master/smtp/test") {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		writeJSON(w, 200, `{"success":true,"message":"sent"}`)
	}))
	defer srv.Close()

	res, err := newTestClient(srv.URL).TestSMTP(context.Background(), cuid, "master", "ops@example.com")
	if err != nil {
		t.Fatalf("TestSMTP: %v", err)
	}
	if !res.Success || res.Message != "sent" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !strings.Contains(body, `"email":"ops@example.com"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestTestIdentityProvider(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/identity-providers/oidc-1/test") {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		writeJSON(w, 200, `{"success":false,"message":"discovery unreachable","details":{"discovery":"timeout"}}`)
	}))
	defer srv.Close()

	res, err := newTestClient(srv.URL).TestIdentityProvider(context.Background(), cuid, "master", "oidc-1", "cid", "csecret")
	if err != nil {
		t.Fatalf("TestIdentityProvider: %v", err)
	}
	if res.Success || res.Details["discovery"] != "timeout" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !strings.Contains(body, `"client_id":"cid"`) || !strings.Contains(body, `"client_secret":"csecret"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestCancelClusterUpgradeAction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/upgrades/cancel") {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, `{"cluster":{"id":"`+cuid+`","name":"prod","status":"available"}}`)
	}))
	defer srv.Close()

	cl, err := newTestClient(srv.URL).CancelClusterUpgrade(context.Background(), cuid)
	if err != nil {
		t.Fatalf("CancelClusterUpgrade: %v", err)
	}
	if cl.Status != "available" || cl.Name != "prod" {
		t.Fatalf("unexpected cluster: %+v", cl)
	}
}

func TestCancelClusterUpgradeConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 409, `{"title":"no upgrade in progress","status":409}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).CancelClusterUpgrade(context.Background(), cuid)
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.StatusCode != 409 {
		t.Fatalf("want 409 APIError, got %v", err)
	}
}
