package skycloak

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestClusterFeaturesRequests(t *testing.T) {
	path := "/clusters/" + cuid + "/features"
	const versionRefusal = "feature 'cimd' needs Keycloak 26.8.0 or newer, but this cluster runs 26.2.4; upgrade the cluster first, then turn the feature on"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("path = %q, want %q", r.URL.Path, path)
		}
		if r.Header.Get("API-Version") != "2026-03-01" || r.Header.Get("API-Key") == "" {
			t.Error("missing or incorrect API headers")
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, `{"features":["cimd","resource-indicators"],"change":{"state":"pending"}}`)
		case http.MethodPut:
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("expected a JSON request")
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			switch string(body) {
			case `{"apply_now":true,"features":[]}`:
				writeJSON(w, 200, `{"features":[],"restarts_keycloak":true,"change":{"state":"applying"}}`)
			case `{"apply_now":false,"features":["cimd"]}`:
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = io.WriteString(w, `{"title":"Validation error","detail":"`+versionRefusal+`"}`)
			default:
				t.Errorf("unexpected request body: %s", body)
				w.WriteHeader(http.StatusBadRequest)
			}
		default:
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()
	client := newTestClient(srv.URL)
	ctx := context.Background()

	got, err := client.GetClusterFeatures(ctx, cuid)
	if err != nil || got.Change != ClusterFeatureChangePending || !slices.Equal(got.Features, []string{"cimd", "resource-indicators"}) {
		t.Fatalf("GetClusterFeatures = %+v, %v", got, err)
	}
	got, err = client.ReplaceClusterFeatures(ctx, cuid, nil, true)
	if err != nil || got.Change != ClusterFeatureChangeApplying || len(got.Features) != 0 {
		t.Fatalf("ReplaceClusterFeatures = %+v, %v", got, err)
	}
	_, err = client.ReplaceClusterFeatures(ctx, cuid, []string{"cimd"}, false)
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.StatusCode != http.StatusUnprocessableEntity || apiErr.Problem.Detail != versionRefusal {
		t.Fatalf("expected version refusal, got %v", err)
	}
}

func TestWaitForClusterFeaturesFailedRestart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clusters/" + cuid + "/features":
			writeJSON(w, 200, `{"features":["cimd"]}`)
		case "/clusters/" + cuid:
			writeJSON(w, 200, `{"status":"failed"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := newTestClient(srv.URL).WaitForClusterFeatures(context.Background(), cuid, []string{"cimd"})
	if err == nil || !strings.Contains(err.Error(), "entered failed state") {
		t.Fatalf("expected restart failure, got %v", err)
	}
}
