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

func TestShouldRetry(t *testing.T) {
	hdr := func(retryAfter string) http.Header {
		h := http.Header{}
		if retryAfter != "" {
			h.Set("Retry-After", retryAfter)
		}
		return h
	}
	cases := []struct {
		name       string
		code       int
		retryAfter string
		want       bool
	}{
		{"429 retried", http.StatusTooManyRequests, "", true},
		{"503 retried", http.StatusServiceUnavailable, "", true},
		{"500 not retried", http.StatusInternalServerError, "", false},
		{"409 with Retry-After retried", http.StatusConflict, "0", true},
		{"409 without Retry-After not retried", http.StatusConflict, "", false},
		{"200 not retried", http.StatusOK, "", false},
	}
	for _, c := range cases {
		got := shouldRetry(&http.Response{StatusCode: c.code, Header: hdr(c.retryAfter)})
		if got != c.want {
			t.Errorf("%s: shouldRetry=%v want %v", c.name, got, c.want)
		}
	}
}

// A 409 that carries Retry-After (e.g. "cluster is updating") is transient and
// gets replayed until it clears.
func TestRetryOn409WithRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0") // cluster updating: retry immediately
			w.WriteHeader(http.StatusConflict)
			return
		}
		writeJSON(w, 200, `[{"id":"`+cuid+`","name":"prod","status":"available"}]`)
	}))
	defer srv.Close()

	clusters, err := newTestClient(srv.URL).ListClusters(context.Background())
	if err != nil {
		t.Fatalf("ListClusters after 409 retry: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls (409 then 200), got %d", calls.Load())
	}
	if len(clusters) != 1 {
		t.Fatalf("unexpected clusters: %+v", clusters)
	}
}

// A 409 without Retry-After is a terminal conflict (e.g. name already taken) and
// must surface to the caller after a single attempt, never loop.
func TestNoRetryOn409WithoutRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).ListClusters(context.Background()); err == nil {
		t.Fatal("expected an error for a terminal 409, got nil")
	}
	if calls.Load() != 1 {
		t.Fatalf("expected exactly 1 call (409 not retried), got %d", calls.Load())
	}
}
