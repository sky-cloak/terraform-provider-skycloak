package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInsightsAndClusterReads(t *testing.T) {
	const gid = "77777777-7777-7777-7777-777777777777"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clusters/" + cuid + "/insights/overview":
			writeJSON(w, 200, `{"total_logins":42,"active_users":7}`)
		case "/clusters/" + cuid + "/credentials":
			writeJSON(w, 200, `{"admin_username":"admin","admin_password":"s3cr3t"}`)
		case "/clusters/" + cuid + "/upgrade-path":
			writeJSON(w, 200, `[{"version":"25.0","required":false},{"version":"26.1","required":true}]`)
		case "/clusters/" + cuid + "/realms/app/groups/" + gid + "/members":
			writeJSON(w, 200, `[{"id":"u1","username":"jdoe","email":"jdoe@example.com","enabled":true}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	raw, err := c.ClusterInsights(context.Background(), cuid, "overview")
	if err != nil || len(raw) == 0 {
		t.Fatalf("ClusterInsights: %q, %v", raw, err)
	}
	creds, err := c.GetClusterCredentials(context.Background(), cuid)
	if err != nil || creds.AdminUsername != "admin" || creds.AdminPassword != "s3cr3t" {
		t.Fatalf("GetClusterCredentials: %+v, %v", creds, err)
	}
	steps, err := c.GetClusterUpgradePath(context.Background(), cuid)
	if err != nil || len(steps) != 2 || !steps[1].Required {
		t.Fatalf("GetClusterUpgradePath: %+v, %v", steps, err)
	}
	members, err := c.ListRealmGroupMembers(context.Background(), cuid, "app", gid)
	if err != nil || len(members) != 1 || members[0].Username != "jdoe" {
		t.Fatalf("ListRealmGroupMembers: %+v, %v", members, err)
	}
}
