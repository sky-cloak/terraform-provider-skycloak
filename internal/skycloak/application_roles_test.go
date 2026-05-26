package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApplicationRolesAndSessions(t *testing.T) {
	base := "/clusters/" + cuid + "/realms/app/applications/web"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == base+"/roles":
			writeJSON(w, 200, `[{"name":"manage-users","description":"Manage users","composite":false,"client_role":true}]`)
		case r.Method == http.MethodPost && r.URL.Path == base+"/roles":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/roles/manage-users":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == base+"/sessions":
			writeJSON(w, 200, `[{"id":"s1","user_id":"u1","username":"jdoe","email":"jdoe@example.com","ip_address":"1.2.3.4","started_at":"2026-01-01T00:00:00Z","last_access_at":"2026-01-01T00:05:00Z"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	roles, err := c.ListApplicationRoles(context.Background(), cuid, "app", "web")
	if err != nil || len(roles) != 1 || !roles[0].ClientRole {
		t.Fatalf("ListApplicationRoles: %+v, %v", roles, err)
	}
	if err := c.AssignApplicationRole(context.Background(), cuid, "app", "web", "manage-users", "realm-management"); err != nil {
		t.Fatalf("AssignApplicationRole: %v", err)
	}
	if err := c.RemoveApplicationRole(context.Background(), cuid, "app", "web", "manage-users", "realm-management"); err != nil {
		t.Fatalf("RemoveApplicationRole: %v", err)
	}
	sessions, err := c.ListApplicationSessions(context.Background(), cuid, "app", "web")
	if err != nil || len(sessions) != 1 || sessions[0].Username != "jdoe" {
		t.Fatalf("ListApplicationSessions: %+v, %v", sessions, err)
	}
}
