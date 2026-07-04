package skycloak

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const siemID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"

const siemDestJSON = `{
	"id":"` + siemID + `","name":"splunk","enabled":true,"type":"http",
	"source":{"type":"keycloak_events","keycloak_event_types":["LOGIN","LOGIN_ERROR"]},
	"batch":{"max_events":500,"max_interval_seconds":30},
	"http":{"url":"https://hec.example.com/services/collector","auth_type":"bearer","has_auth_credentials":true,"header_names":["X-Env"]},
	"health_status":"healthy","failure_count":0,
	"total_events_sent":100,"total_logs_sent":0,"total_bytes_sent":2048,
	"created_at":"2026-07-01T00:00:00Z","updated_at":"2026-07-02T00:00:00Z"
}`

func TestSIEMDestinationCRUD(t *testing.T) {
	var createBody, updateBody string
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/siem/destinations":
			writeJSON(w, 200, "["+siemDestJSON+"]")
		case r.Method == http.MethodPost && r.URL.Path == "/siem/destinations":
			raw, _ := io.ReadAll(r.Body)
			createBody = string(raw)
			writeJSON(w, 201, siemDestJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/siem/destinations/"+siemID:
			writeJSON(w, 200, siemDestJSON)
		case r.Method == http.MethodPatch && r.URL.Path == "/siem/destinations/"+siemID:
			raw, _ := io.ReadAll(r.Body)
			updateBody = string(raw)
			writeJSON(w, 200, siemDestJSON)
		case r.Method == http.MethodDelete && r.URL.Path == "/siem/destinations/"+siemID:
			deleted = true
			w.WriteHeader(204)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	list, err := c.ListSIEMDestinations(context.Background())
	if err != nil || len(list) != 1 || list[0].Name != "splunk" {
		t.Fatalf("ListSIEMDestinations: %+v, %v", list, err)
	}
	if list[0].HTTP == nil || !list[0].HTTP.HasAuthCredentials || list[0].HTTP.HeaderNames[0] != "X-Env" {
		t.Fatalf("http config not mapped: %+v", list[0].HTTP)
	}
	if list[0].Batch == nil || list[0].Batch.MaxEvents != 500 {
		t.Fatalf("batch not mapped: %+v", list[0].Batch)
	}

	enabled := true
	created, err := c.CreateSIEMDestination(context.Background(), CreateSIEMDestinationRequest{
		Name: "splunk", Enabled: &enabled, Type: "http",
		Source: SIEMSource{Type: "keycloak_events", KeycloakEventTypes: []string{"LOGIN"}},
		HTTP:   &SIEMHTTP{URL: "https://hec.example.com/services/collector", AuthType: "bearer", BearerToken: "tok-secret"},
	})
	if err != nil || created.ID != siemID {
		t.Fatalf("CreateSIEMDestination: %+v, %v", created, err)
	}
	if !strings.Contains(createBody, `"bearer_token":"tok-secret"`) {
		t.Fatalf("create body missing write-only secret: %s", createBody)
	}

	got, err := c.GetSIEMDestination(context.Background(), siemID)
	if err != nil || got.HealthStatus != "healthy" || got.Source.Type != "keycloak_events" {
		t.Fatalf("GetSIEMDestination: %+v, %v", got, err)
	}

	_, err = c.UpdateSIEMDestination(context.Background(), siemID, CreateSIEMDestinationRequest{
		Name: "splunk-2", Type: "http",
		Source: SIEMSource{Type: "keycloak_events", KeycloakEventTypes: []string{"LOGIN"}},
		HTTP:   &SIEMHTTP{URL: "https://hec2.example.com", AuthType: "none"},
	})
	if err != nil {
		t.Fatalf("UpdateSIEMDestination: %v", err)
	}
	var upd map[string]any
	if err := json.Unmarshal([]byte(updateBody), &upd); err != nil || upd["name"] != "splunk-2" {
		t.Fatalf("unexpected update body: %s (%v)", updateBody, err)
	}

	if err := c.DeleteSIEMDestination(context.Background(), siemID); err != nil || !deleted {
		t.Fatalf("DeleteSIEMDestination: %v (called=%v)", err, deleted)
	}
}

func TestSIEMDestinationTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/siem/destinations/"+siemID+"/test" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, `{"success":false,"message":"connection refused"}`)
	}))
	defer srv.Close()

	res, err := newTestClient(srv.URL).TestSIEMDestination(context.Background(), siemID)
	if err != nil {
		t.Fatalf("TestSIEMDestination: %v", err)
	}
	if res.Success || res.Message != "connection refused" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestSIEMPlanGatePassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 403, `{"title":"plan required","detail":"SIEM forwarding requires the Enterprise plan","status":403}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).ListSIEMDestinations(context.Background())
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.StatusCode != 403 || !strings.Contains(apiErr.Error(), "Enterprise plan") {
		t.Fatalf("want 403 passthrough with detail, got %v", err)
	}
}

func TestSIEMS3SecretsSent(t *testing.T) {
	var createBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		createBody = string(raw)
		writeJSON(w, 201, siemDestJSON)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).CreateSIEMDestination(context.Background(), CreateSIEMDestinationRequest{
		Name: "s3-dest", Type: "s3",
		Source: SIEMSource{Type: "skycloak_audit"},
		S3: &SIEMS3{
			Bucket: "logs", Region: "us-east-1", AuthType: "access_key",
			AccessKeyID: "AKIA123", SecretAccessKey: "shhh",
		},
	})
	if err != nil {
		t.Fatalf("CreateSIEMDestination: %v", err)
	}
	if !strings.Contains(createBody, `"access_key_id":"AKIA123"`) || !strings.Contains(createBody, `"secret_access_key":"shhh"`) {
		t.Fatalf("s3 secrets missing from body: %s", createBody)
	}
}
