package skycloak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestClusterPrivateLinkLifecycle(t *testing.T) {
	ids := []string{"22222222-2222-2222-2222-222222222222"}
	hosts := []string{"console.example.com"}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/clusters/"+cuid+"/private-link" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("API-Version") != DefaultAPIVersion {
			t.Error("missing API version")
		}
		if _, ok := r.Header["Api-Key"]; !ok {
			t.Error("missing API key header")
		}
		if r.Header.Get("User-Agent") != "skycloak-go/dev" {
			t.Error("missing user agent")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("missing JSON accept header")
		}
		result := map[string]any{"enabled": true, "azure_subscription_ids": ids, "alias": "example.alias", "hostnames": hosts, "status": "waiting"}
		switch calls {
		case 1:
			if r.Method != http.MethodGet {
				t.Errorf("method = %s", r.Method)
			}
		case 2, 3, 4:
			if r.Method != http.MethodPut {
				t.Errorf("method = %s", r.Method)
			}
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			var enabled bool
			if err := json.Unmarshal(body["enabled"], &enabled); err != nil {
				t.Error(err)
			}
			if enabled != (calls == 2) {
				t.Errorf("enabled = %v on call %d", enabled, calls)
			}
			switch calls {
			case 2:
				var sent []string
				if err := json.Unmarshal(body["azure_subscription_ids"], &sent); err != nil {
					t.Error(err)
				}
				if !reflect.DeepEqual(sent, ids) {
					t.Errorf("subscriptions = %v", sent)
				}
				var sentHosts []string
				if err := json.Unmarshal(body["hostnames"], &sentHosts); err != nil {
					t.Error(err)
				}
				if !reflect.DeepEqual(sentHosts, hosts) {
					t.Errorf("hostnames = %v", sentHosts)
				}
			case 3:
				if string(body["azure_subscription_ids"]) != "[]" {
					t.Error("empty IDs must clear the list")
				}
				if string(body["hostnames"]) != "[]" {
					t.Error("empty hostnames must clear the list")
				}
			case 4:
				if _, ok := body["azure_subscription_ids"]; ok {
					t.Error("disable must omit IDs to preserve them")
				}
				if _, ok := body["hostnames"]; ok {
					t.Error("disable must omit hostnames to preserve them")
				}
			}
			result["enabled"] = enabled
			if !enabled {
				delete(result, "alias")
			}
		default:
			t.Error("unexpected request")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	client := New(srv.URL, "", "")
	link, err := client.GetClusterPrivateLink(context.Background(), cuid)
	if err != nil {
		t.Fatal(err)
	}
	if !link.Enabled || !reflect.DeepEqual(link.AzureSubscriptionIDs, ids) || link.Alias == nil || *link.Alias != "example.alias" || !reflect.DeepEqual(link.Hostnames, hosts) {
		t.Fatalf("unexpected link: %+v", link)
	}
	for _, tc := range []struct {
		enabled bool
		ids     []string
		hosts   []string
	}{{true, ids, hosts}, {false, []string{}, []string{}}, {false, nil, nil}} {
		link, err = client.UpdateClusterPrivateLink(context.Background(), cuid, tc.enabled, tc.ids, tc.hosts)
		if err != nil {
			t.Fatal(err)
		}
		if link.Enabled != tc.enabled || (!tc.enabled && link.Alias != nil) {
			t.Fatalf("unexpected update: %+v", link)
		}
	}
	if calls != 4 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestClusterPrivateLinkErrors(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, status := range []int{403, 404, 409, 422} {
			t.Run(fmt.Sprintf("%s/%d", method, status), func(t *testing.T) {
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Method != method {
						t.Errorf("method = %s", r.Method)
					}
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(status)
					_, _ = fmt.Fprintf(w, `{"title":"refused","status":%d,"detail":"private link cannot be changed"}`, status)
				}))
				defer srv.Close()
				client := New(srv.URL, "", "")
				var err error
				if method == http.MethodGet {
					_, err = client.GetClusterPrivateLink(context.Background(), cuid)
				} else {
					_, err = client.UpdateClusterPrivateLink(context.Background(), cuid, false, nil, nil)
				}
				var apiErr *APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("want API error, got %v", err)
				}
				if apiErr.StatusCode != status || apiErr.Problem.Detail != "private link cannot be changed" {
					t.Fatalf("unexpected API error: %v", apiErr)
				}
				if IsNotFound(err) != (status == 404) {
					t.Fatalf("IsNotFound = %v", IsNotFound(err))
				}
				if calls != 1 {
					t.Errorf("terminal error retried %d times", calls)
				}
			})
		}
	}
}
