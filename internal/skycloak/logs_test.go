package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClusterLogsEventsBuild(t *testing.T) {
	const bid = "88888888-8888-8888-8888-888888888888"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clusters/" + cuid + "/logs":
			writeJSON(w, 200, `[{"category":"org.keycloak","level":"INFO","message":"started","source":"kc","thread_name":"main","timestamp":"2026-01-01T00:00:00Z"}]`)
		case "/clusters/" + cuid + "/security-logs":
			writeJSON(w, 200, `[{"id":"s1","type":"waf","action":"blocked","source_ip":"1.2.3.4","country":"US","method":"GET","uri":"/","message":"blocked","matched_rules":[],"anomaly_score":null,"attack_type":null,"timestamp":"2026-01-01T00:00:00Z"}]`)
		case "/clusters/" + cuid + "/events":
			writeJSON(w, 200, `[{"category":"user","type":"LOGIN","realm_name":"app","client_id":"web","username":"jdoe","ip_address":"1.2.3.4","timestamp":"2026-01-01T00:00:00Z"}]`)
		case "/clusters/" + cuid + "/builds/" + bid:
			writeJSON(w, 200, `{"id":"`+bid+`","status":"completed","phase":"done","progress":100,"error":null,"logs":["step 1","step 2"],"started_at":"2026-01-01T00:00:00Z","completed_at":"2026-01-01T00:05:00Z"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if logs, err := c.ListClusterLogs(context.Background(), cuid, LogQuery{Limit: 10, Level: "INFO"}); err != nil || len(logs) != 1 || logs[0].Level != "INFO" {
		t.Fatalf("ListClusterLogs: %+v, %v", logs, err)
	}
	if sl, err := c.ListClusterSecurityLogs(context.Background(), cuid, 10, ""); err != nil || len(sl) != 1 || sl[0].Action != "blocked" {
		t.Fatalf("ListClusterSecurityLogs: %+v, %v", sl, err)
	}
	if ev, err := c.ListClusterEvents(context.Background(), cuid, EventQuery{Limit: 10, Category: "user"}); err != nil || len(ev) != 1 || ev[0].Username != "jdoe" {
		t.Fatalf("ListClusterEvents: %+v, %v", ev, err)
	}
}
