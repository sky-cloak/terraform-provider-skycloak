package skycloak

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClusterSecurityGetUpdate(t *testing.T) {
	path := "/clusters/" + cuid + "/security"
	// Existing config has a captcha block that must survive the update.
	current := `{"captcha":{"enabled":true},"ip_access_control":{"path_rules":[{"path":"/admin","allowed_ips":["1.2.3.4"]}]}}`
	var putBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, current)
		case http.MethodPut, http.MethodPatch:
			raw, _ := io.ReadAll(r.Body)
			putBody = string(raw)
			writeJSON(w, 200, putBody)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.GetClusterSecurity(context.Background(), cuid)
	if err != nil || got.IPAccessControl == nil || len(got.IPAccessControl.PathRules) != 1 {
		t.Fatalf("GetClusterSecurity: %+v, %v", got, err)
	}

	_, err = c.UpdateClusterSecurity(context.Background(), cuid, &ClusterSecurity{
		WAF:          &WAF{Enabled: true, Mode: "block", Preset: "owasp_top_10", ParanoiaLevel: 1},
		GeoBlocking:  &GeoBlocking{Enabled: true, Mode: "blocklist", Countries: []string{"RU", "KP"}},
		RateLimiting: &RateLimiting{Enabled: true, GlobalRPM: 6000},
	})
	if err != nil {
		t.Fatalf("UpdateClusterSecurity: %v", err)
	}
	// CAPTCHA must be preserved, and the new WAF/geo/rate config must be present.
	if !strings.Contains(putBody, `"captcha"`) {
		t.Fatalf("update body dropped captcha: %s", putBody)
	}
	if !strings.Contains(putBody, `"waf"`) || !strings.Contains(putBody, `"geo_blocking"`) {
		t.Fatalf("update body missing managed sections: %s", putBody)
	}
	// The old ip_access_control should have been cleared (not in the new desired state).
	if strings.Contains(putBody, `"/admin"`) {
		t.Fatalf("update body retained a stale ip rule: %s", putBody)
	}
}

func TestClusterSecurityManagedCAPTCHA(t *testing.T) {
	path := "/clusters/" + cuid + "/security"
	current := `{"captcha":{"enabled":true,"enabled_realms":["old"]},"waf":{"enabled":true,"mode":"block","preset":"custom","paranoia_level":1}}`
	var putBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, current)
		case http.MethodPut, http.MethodPatch:
			raw, _ := io.ReadAll(r.Body)
			putBody = string(raw)
			writeJSON(w, 200, putBody)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.GetClusterSecurity(context.Background(), cuid)
	if err != nil || got.CAPTCHA == nil || !got.CAPTCHA.Enabled || got.CAPTCHA.EnabledRealms[0] != "old" {
		t.Fatalf("GetClusterSecurity captcha: %+v, %v", got, err)
	}

	// Managed block replaces the server captcha config instead of preserving it.
	_, err = c.UpdateClusterSecurity(context.Background(), cuid, &ClusterSecurity{
		CAPTCHA: &CAPTCHA{Enabled: false, EnabledRealms: []string{"customers"}},
	})
	if err != nil {
		t.Fatalf("UpdateClusterSecurity: %v", err)
	}
	if !strings.Contains(putBody, `"enabled_realms":["customers"]`) || strings.Contains(putBody, `"old"`) {
		t.Fatalf("managed captcha not applied: %s", putBody)
	}
}
