package skycloak

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const ofgaID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"

func ofgaJSON(status, extra string) string {
	return `{"id":"` + ofgaID + `","cluster_id":"` + cuid + `","realm":"my-service","status":"` + status + `",` + extra +
		`"api_url":"https://openfga-7c9e6679.eu.skycloak.io","issuer":"https://auth.example.com/realms/my-service",` +
		`"audience":"https://openfga-7c9e6679.eu.skycloak.io","created_at":"2026-11-05T10:00:00Z","updated_at":"2026-11-05T10:02:10Z"}`
}

func TestOpenFGAInstanceLifecycle(t *testing.T) {
	base := "/clusters/" + cuid + "/openfga-instances"
	var createBody string
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("API-Version") == "" {
			t.Errorf("missing API-Version header")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == base:
			writeJSON(w, 200, `{"instances":[`+ofgaJSON("ready", "")+`]}`)
		case r.Method == http.MethodPost && r.URL.Path == base:
			raw, _ := io.ReadAll(r.Body)
			createBody = string(raw)
			writeJSON(w, 202, ofgaJSON("provisioning", ""))
		case r.Method == http.MethodGet && r.URL.Path == base+"/"+ofgaID:
			writeJSON(w, 200, ofgaJSON("ready", ""))
		case r.Method == http.MethodDelete && r.URL.Path == base+"/"+ofgaID:
			deleted = true
			writeJSON(w, 202, ofgaJSON("deleting", ""))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	ctx := context.Background()

	list, err := c.ListOpenFGAInstances(ctx, cuid)
	if err != nil || len(list) != 1 || list[0].Realm != "my-service" || list[0].ID != ofgaID {
		t.Fatalf("unexpected list: %+v (%v)", list, err)
	}

	created, err := c.CreateOpenFGAInstance(ctx, cuid, "my-service")
	if err != nil {
		t.Fatalf("CreateOpenFGAInstance: %v", err)
	}
	if created.Status != OpenFGAStatusProvisioning || created.ID != ofgaID {
		t.Fatalf("unexpected create result: %+v", created)
	}
	var sent map[string]string
	if err := json.Unmarshal([]byte(createBody), &sent); err != nil || sent["realm"] != "my-service" {
		t.Fatalf("unexpected create body: %s (%v)", createBody, err)
	}

	got, err := c.GetOpenFGAInstance(ctx, cuid, ofgaID)
	if err != nil {
		t.Fatalf("GetOpenFGAInstance: %v", err)
	}
	if got.APIURL == "" || got.Issuer == "" || got.Audience == "" || got.CreatedAt != "2026-11-05T10:00:00Z" {
		t.Fatalf("unexpected instance: %+v", got)
	}

	if err := c.DeleteOpenFGAInstance(ctx, cuid, ofgaID); err != nil || !deleted {
		t.Fatalf("DeleteOpenFGAInstance: %v (called=%v)", err, deleted)
	}
}

func TestOpenFGAInstanceErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			writeJSON(w, 403, `{"title":"Forbidden","status":403}`)
		default:
			writeJSON(w, 404, `{"title":"Not Found","status":404}`)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	ctx := context.Background()

	_, err := c.CreateOpenFGAInstance(ctx, cuid, "r")
	if e, ok := AsAPIError(err); !ok || e.StatusCode != 403 {
		t.Fatalf("want 403 API error, got %v", err)
	}
	if _, err := c.GetOpenFGAInstance(ctx, cuid, ofgaID); !IsNotFound(err) {
		t.Fatalf("want not found, got %v", err)
	}
	if _, err := c.GetOpenFGAInstance(ctx, cuid, "not-a-uuid"); err == nil || !strings.Contains(err.Error(), "invalid OpenFGA instance ID") {
		t.Fatalf("want invalid ID error, got %v", err)
	}
}

func TestWaitForOpenFGAInstanceReady(t *testing.T) {
	old := openFGAPollInterval
	openFGAPollInterval = time.Millisecond
	defer func() { openFGAPollInterval = old }()

	for _, tc := range []struct {
		name    string
		states  []string
		wantErr string
	}{
		{"becomes ready", []string{"provisioning", "provisioning", "ready"}, ""},
		{"fails", []string{"provisioning", "failed"}, "boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				s := tc.states[min(n, len(tc.states)-1)]
				n++
				extra := ""
				if s == "failed" {
					extra = `"status_message":"boom",`
				}
				writeJSON(w, 200, ofgaJSON(s, extra))
			}))
			defer srv.Close()
			inst, err := newTestClient(srv.URL).WaitForOpenFGAInstanceReady(context.Background(), cuid, ofgaID)
			if tc.wantErr == "" {
				if err != nil || inst.Status != OpenFGAStatusReady {
					t.Fatalf("want ready, got %+v (%v)", inst, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestWaitForOpenFGAInstanceGone(t *testing.T) {
	old := openFGAPollInterval
	openFGAPollInterval = time.Millisecond
	defer func() { openFGAPollInterval = old }()

	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n < 3 {
			writeJSON(w, 200, ofgaJSON("deleting", ""))
			return
		}
		writeJSON(w, 404, `{"title":"Not Found","status":404}`)
	}))
	defer srv.Close()
	if err := newTestClient(srv.URL).WaitForOpenFGAInstanceGone(context.Background(), cuid, ofgaID); err != nil {
		t.Fatalf("WaitForOpenFGAInstanceGone: %v", err)
	}
	if n != 3 {
		t.Fatalf("polled %d times, want 3", n)
	}
}
