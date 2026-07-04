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

const whID = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

const webhookJSON = `{
	"id":"` + whID + `","name":"ops","url":"https://hooks.example.com/skycloak","enabled":true,
	"source":"keycloak","event_types":["LOGIN_ERROR"],
	"has_authorization_header":true,"has_signing_secret":true,
	"created_at":"2026-07-01T00:00:00Z","updated_at":"2026-07-02T00:00:00Z"
}`

func TestWebhookSubscriptionCRUD(t *testing.T) {
	var createBody, updateBody string
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/webhooks":
			writeJSON(w, 200, "["+webhookJSON+"]")
		case r.Method == http.MethodPost && r.URL.Path == "/webhooks":
			raw, _ := io.ReadAll(r.Body)
			createBody = string(raw)
			writeJSON(w, 201, webhookJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/webhooks/"+whID:
			writeJSON(w, 200, webhookJSON)
		case r.Method == http.MethodPatch && r.URL.Path == "/webhooks/"+whID:
			raw, _ := io.ReadAll(r.Body)
			updateBody = string(raw)
			writeJSON(w, 200, webhookJSON)
		case r.Method == http.MethodDelete && r.URL.Path == "/webhooks/"+whID:
			deleted = true
			w.WriteHeader(204)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	list, err := c.ListWebhookSubscriptions(context.Background())
	if err != nil || len(list) != 1 || !list[0].HasSigningSecret {
		t.Fatalf("ListWebhookSubscriptions: %+v, %v", list, err)
	}

	created, err := c.CreateWebhookSubscription(context.Background(), WebhookSubscriptionRequest{
		Name: "ops", URL: "https://hooks.example.com/skycloak", Source: "keycloak",
		EventTypes: []string{"LOGIN_ERROR"}, SigningSecret: "whsec_123", AuthorizationHeader: "Bearer abc",
	})
	if err != nil || created.ID != whID {
		t.Fatalf("CreateWebhookSubscription: %+v, %v", created, err)
	}
	if !strings.Contains(createBody, `"signing_secret":"whsec_123"`) || !strings.Contains(createBody, `"authorization_header":"Bearer abc"`) {
		t.Fatalf("create body missing secrets: %s", createBody)
	}

	got, err := c.GetWebhookSubscription(context.Background(), whID)
	if err != nil || got.Name != "ops" {
		t.Fatalf("GetWebhookSubscription: %+v, %v", got, err)
	}

	_, err = c.UpdateWebhookSubscription(context.Background(), whID, WebhookSubscriptionRequest{
		Name: "ops-2", URL: "https://hooks.example.com/v2", Source: "keycloak", EventTypes: []string{"LOGIN"},
	})
	if err != nil {
		t.Fatalf("UpdateWebhookSubscription: %v", err)
	}
	var upd map[string]any
	if err := json.Unmarshal([]byte(updateBody), &upd); err != nil {
		t.Fatalf("update body not json: %v", err)
	}
	if upd["name"] != "ops-2" {
		t.Fatalf("unexpected update body: %s", updateBody)
	}
	if _, present := upd["signing_secret"]; present {
		t.Fatalf("empty signing_secret must be omitted (retain), body: %s", updateBody)
	}
	// Unscoped subscription: cluster_id/realm_id sent as explicit nulls to clear.
	if v, present := upd["cluster_id"]; !present || v != nil {
		t.Fatalf("cluster_id should be an explicit null, body: %s", updateBody)
	}

	if err := c.DeleteWebhookSubscription(context.Background(), whID); err != nil || !deleted {
		t.Fatalf("DeleteWebhookSubscription: %v (called=%v)", err, deleted)
	}
}

func TestWebhookSubscriptionTest(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/webhooks/"+whID+"/test" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		writeJSON(w, 200, `{"delivery_id":"d-1","success":true,"response_code":200,"duration_ms":42,"attempted_at":"2026-07-04T00:00:00Z"}`)
	}))
	defer srv.Close()

	res, err := newTestClient(srv.URL).TestWebhookSubscription(context.Background(), whID, "LOGIN_ERROR")
	if err != nil {
		t.Fatalf("TestWebhookSubscription: %v", err)
	}
	if !res.Success || res.ResponseCode != 200 || res.DeliveryID != "d-1" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !strings.Contains(body, `"event_type":"LOGIN_ERROR"`) {
		t.Fatalf("unexpected request body: %s", body)
	}
}

func TestListWebhookEventTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/webhook-event-types" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, `[{"type":"LOGIN_ERROR","category":"keycloak","description":"Failed login","deprecated":false,"sample_payload":null}]`)
	}))
	defer srv.Close()

	types, err := newTestClient(srv.URL).ListWebhookEventTypes(context.Background())
	if err != nil || len(types) != 1 || types[0].Type != "LOGIN_ERROR" || types[0].Category != "keycloak" {
		t.Fatalf("ListWebhookEventTypes: %+v, %v", types, err)
	}
}
