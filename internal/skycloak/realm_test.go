package skycloak

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealmFrontendURL(t *testing.T) {
	path := "/clusters/" + cuid + "/realms/app"
	const base = `{"id":"123e4567-e89b-12d3-a456-426614174000","name":"app","display_name":"App","enabled":true,"ssl_required":"external","registration_allowed":false,"registration_email_as_username":false,"login_with_email_allowed":false,"duplicate_emails_allowed":false`
	var body map[string]json.RawMessage
	var reply string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, reply)
		case http.MethodPatch:
			raw, _ := io.ReadAll(r.Body)
			body = nil
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("patch body is not JSON: %v", err)
			}
			writeJSON(w, 200, reply)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	ctx := context.Background()

	url := "https://login.example.com"
	cleared := ""
	cases := []struct {
		name     string
		set      *string
		wantBody string
		wantSent bool
	}{
		{"omitted when unset", nil, "", false},
		{"sent when set", &url, `"https://login.example.com"`, true},
		{"null when cleared", &cleared, `null`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reply = base + `,"frontend_url":null}`
			if tc.set != nil && *tc.set != "" {
				reply = base + `,"frontend_url":"` + *tc.set + `"}`
			}
			got, err := c.UpdateRealm(ctx, cuid, "app", Realm{Name: "app", Enabled: true, FrontendURL: tc.set})
			if err != nil {
				t.Fatalf("UpdateRealm: %v", err)
			}
			raw, sent := body["frontend_url"]
			if sent != tc.wantSent || (sent && string(raw) != tc.wantBody) {
				t.Fatalf("frontend_url in body = %s (sent %v), want %s (sent %v)", raw, sent, tc.wantBody, tc.wantSent)
			}
			if tc.set != nil && *tc.set != "" {
				if got.FrontendURL == nil || *got.FrontendURL != *tc.set {
					t.Fatalf("FrontendURL = %v, want %q", got.FrontendURL, *tc.set)
				}
			} else if got.FrontendURL != nil {
				t.Fatalf("FrontendURL = %q, want nil", *got.FrontendURL)
			}
		})
	}

	reply = base + `}`
	got, err := c.GetRealm(ctx, cuid, "app")
	if err != nil || got.FrontendURL != nil {
		t.Fatalf("GetRealm without the field: %+v, %v", got, err)
	}
}
