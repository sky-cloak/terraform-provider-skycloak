package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestRetryOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0") // first call: rate limited, retry immediately
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(w, 200, `[{"id":"`+cuid+`","name":"prod","status":"available"}]`)
	}))
	defer srv.Close()

	clusters, err := newTestClient(srv.URL).ListClusters(context.Background())
	if err != nil {
		t.Fatalf("ListClusters after retry: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls (429 then 200), got %d", calls.Load())
	}
	if len(clusters) != 1 {
		t.Fatalf("unexpected clusters: %+v", clusters)
	}
}

func TestBackoffDelayHonorsRetryAfter(t *testing.T) {
	if d := backoffDelay(0, "5"); d.Seconds() != 5 {
		t.Fatalf("Retry-After: got %v, want 5s", d)
	}
	if d := backoffDelay(2, ""); d.Seconds() != 4 {
		t.Fatalf("exponential: got %v, want 4s", d)
	}
}
