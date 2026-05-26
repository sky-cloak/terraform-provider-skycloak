package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	groupUID = "77777777-7777-7777-7777-777777777777"
	userID   = "user-123"
)

func TestRealmRoleCRUD(t *testing.T) {
	base := "/clusters/" + cuid + "/realms/app/roles"
	body := `{"name":"admin","description":"Administrators","composite":false,"client_role":false}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == base:
			writeJSON(w, 200, "["+body+"]")
		case r.Method == http.MethodPost && r.URL.Path == base:
			writeJSON(w, http.StatusCreated, body)
		case r.Method == http.MethodGet && r.URL.Path == base+"/admin":
			writeJSON(w, 200, body)
		case (r.Method == http.MethodPatch || r.Method == http.MethodPut) && r.URL.Path == base+"/admin":
			writeJSON(w, 200, body)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/admin":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	created, err := c.CreateRealmRole(context.Background(), cuid, "app", "admin", "Administrators")
	if err != nil || created.Name != "admin" {
		t.Fatalf("CreateRealmRole: %+v, %v", created, err)
	}
	if roles, err := c.ListRealmRoles(context.Background(), cuid, "app"); err != nil || len(roles) != 1 {
		t.Fatalf("ListRealmRoles: %+v, %v", roles, err)
	}
	if _, err := c.GetRealmRole(context.Background(), cuid, "app", "admin"); err != nil {
		t.Fatalf("GetRealmRole: %v", err)
	}
	if _, err := c.UpdateRealmRole(context.Background(), cuid, "app", "admin", "Admins"); err != nil {
		t.Fatalf("UpdateRealmRole: %v", err)
	}
	if err := c.DeleteRealmRole(context.Background(), cuid, "app", "admin"); err != nil {
		t.Fatalf("DeleteRealmRole: %v", err)
	}
}

func TestRealmGroupCRUD(t *testing.T) {
	base := "/clusters/" + cuid + "/realms/app/groups"
	body := `{"id":"` + groupUID + `","name":"eng","path":"/eng","sub_group_count":0}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == base:
			writeJSON(w, 200, "["+body+"]")
		case r.Method == http.MethodPost && r.URL.Path == base:
			writeJSON(w, http.StatusCreated, body)
		case r.Method == http.MethodGet && r.URL.Path == base+"/"+groupUID:
			writeJSON(w, 200, body)
		case (r.Method == http.MethodPatch || r.Method == http.MethodPut) && r.URL.Path == base+"/"+groupUID:
			writeJSON(w, 200, body)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/"+groupUID:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	created, err := c.CreateRealmGroup(context.Background(), cuid, "app", "eng", "")
	if err != nil || created.ID != groupUID || created.Path != "/eng" {
		t.Fatalf("CreateRealmGroup: %+v, %v", created, err)
	}
	if groups, err := c.ListRealmGroups(context.Background(), cuid, "app"); err != nil || len(groups) != 1 {
		t.Fatalf("ListRealmGroups: %+v, %v", groups, err)
	}
	if _, err := c.GetRealmGroup(context.Background(), cuid, "app", groupUID); err != nil {
		t.Fatalf("GetRealmGroup: %v", err)
	}
	if _, err := c.UpdateRealmGroup(context.Background(), cuid, "app", groupUID, "engineering"); err != nil {
		t.Fatalf("UpdateRealmGroup: %v", err)
	}
	if err := c.DeleteRealmGroup(context.Background(), cuid, "app", groupUID); err != nil {
		t.Fatalf("DeleteRealmGroup: %v", err)
	}
}

func TestRealmUserCRUDAndAssignments(t *testing.T) {
	base := "/clusters/" + cuid + "/realms/app/users"
	user := `{"id":"` + userID + `","username":"jdoe","email":"jdoe@example.com","enabled":true,"email_verified":false,"first_name":"J","last_name":"Doe"}`
	role := `{"name":"admin","description":null,"composite":false,"client_role":false}`
	group := `{"id":"` + groupUID + `","name":"eng","path":"/eng"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPost && p == base:
			writeJSON(w, http.StatusCreated, user)
		case r.Method == http.MethodGet && p == base+"/"+userID:
			writeJSON(w, 200, user)
		case (r.Method == http.MethodPatch || r.Method == http.MethodPut) && p == base+"/"+userID:
			writeJSON(w, 200, user)
		case r.Method == http.MethodDelete && p == base+"/"+userID:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && p == base+"/"+userID+"/roles":
			writeJSON(w, 200, "["+role+"]")
		case r.Method == http.MethodPost && p == base+"/"+userID+"/roles":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && p == base+"/"+userID+"/roles/admin":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && p == base+"/"+userID+"/groups":
			writeJSON(w, 200, "["+group+"]")
		case (r.Method == http.MethodPut || r.Method == http.MethodPost) && p == base+"/"+userID+"/groups/"+groupUID:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && p == base+"/"+userID+"/groups/"+groupUID:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	u, err := c.CreateRealmUser(context.Background(), cuid, "app", CreateRealmUserRequest{Username: "jdoe", Email: "jdoe@example.com", FirstName: "J", LastName: "Doe", Enabled: true, TemporaryPassword: "password1"})
	if err != nil || u.ID != userID {
		t.Fatalf("CreateRealmUser: %+v, %v", u, err)
	}
	if _, err := c.GetRealmUser(context.Background(), cuid, "app", userID); err != nil {
		t.Fatalf("GetRealmUser: %v", err)
	}
	if _, err := c.UpdateRealmUser(context.Background(), cuid, "app", userID, CreateRealmUserRequest{Email: "jdoe@example.com", Enabled: true}, true); err != nil {
		t.Fatalf("UpdateRealmUser: %v", err)
	}
	if err := c.AssignRealmUserRole(context.Background(), cuid, "app", userID, "admin"); err != nil {
		t.Fatalf("AssignRealmUserRole: %v", err)
	}
	if roles, err := c.ListRealmUserRoles(context.Background(), cuid, "app", userID); err != nil || len(roles) != 1 {
		t.Fatalf("ListRealmUserRoles: %+v, %v", roles, err)
	}
	if err := c.RemoveRealmUserRole(context.Background(), cuid, "app", userID, "admin"); err != nil {
		t.Fatalf("RemoveRealmUserRole: %v", err)
	}
	if err := c.AddRealmUserToGroup(context.Background(), cuid, "app", userID, groupUID); err != nil {
		t.Fatalf("AddRealmUserToGroup: %v", err)
	}
	if groups, err := c.ListRealmUserGroups(context.Background(), cuid, "app", userID); err != nil || len(groups) != 1 {
		t.Fatalf("ListRealmUserGroups: %+v, %v", groups, err)
	}
	if err := c.RemoveRealmUserFromGroup(context.Background(), cuid, "app", userID, groupUID); err != nil {
		t.Fatalf("RemoveRealmUserFromGroup: %v", err)
	}
	if err := c.DeleteRealmUser(context.Background(), cuid, "app", userID); err != nil {
		t.Fatalf("DeleteRealmUser: %v", err)
	}
}
