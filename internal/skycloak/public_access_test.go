package skycloak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const publicAccessClusterBody = `{"id":"` + cuid + `","name":"c","type":"DEDICATED","size":"SMALL","version":"26.0.0","location":"us","status":"READY","url":"https://c.example","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","http_relative_path":"","management_mode":"FULLY_MANAGED"%s}`

func TestClusterPublicAccess(t *testing.T) {
	publicAccess := `,"public_access":false`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/clusters/"+cuid {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method == http.MethodPatch || r.Method == http.MethodPut {
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body) != 1 {
				t.Errorf("only public_access must be sent, got %v", body)
			}
			publicAccess = `,"public_access":` + string(body["public_access"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, publicAccessClusterBody, publicAccess)
	}))
	defer srv.Close()
	client := New(srv.URL, "", "")
	ctx := context.Background()
	got, err := client.GetClusterPublicAccess(ctx, cuid)
	if err != nil || got {
		t.Fatalf("get = %v, %v", got, err)
	}
	got, err = client.SetClusterPublicAccess(ctx, cuid, true)
	if err != nil || !got {
		t.Fatalf("set true = %v, %v", got, err)
	}
	got, err = client.SetClusterPublicAccess(ctx, cuid, false)
	if err != nil || got {
		t.Fatalf("set false = %v, %v", got, err)
	}
	publicAccess = ""
	got, err = client.GetClusterPublicAccess(ctx, cuid)
	if err != nil || !got {
		t.Fatalf("absent setting must read as public: %v, %v", got, err)
	}
}

func TestClusterPublicAccessErrors(t *testing.T) {
	for _, set := range []bool{false, true} {
		for _, status := range []int{403, 404, 409, 422} {
			t.Run(fmt.Sprintf("set=%v/%d", set, status), func(t *testing.T) {
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls++
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(status)
					_, _ = fmt.Fprintf(w, `{"title":"refused","status":%d}`, status)
				}))
				defer srv.Close()
				client := New(srv.URL, "", "")
				var err error
				if set {
					_, err = client.SetClusterPublicAccess(context.Background(), cuid, false)
				} else {
					_, err = client.GetClusterPublicAccess(context.Background(), cuid)
				}
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
					t.Fatalf("want API error %d, got %v", status, err)
				}
				if IsNotFound(err) != (status == 404) || calls != 1 {
					t.Fatalf("IsNotFound = %v, calls = %d", IsNotFound(err), calls)
				}
			})
		}
	}
}
