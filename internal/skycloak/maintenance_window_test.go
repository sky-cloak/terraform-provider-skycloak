package skycloak

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaintenanceWindowLifecycle(t *testing.T) {
	path := "/clusters/" + cuid + "/maintenance-window"
	window := `{"enabled":true,"days_of_week":[2,4],"start_local":"02:00","end_local":"05:00","timezone":"Europe/Berlin"}`
	var putBody string
	var deleted bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("API-Version"); got != "2026-03-01" {
			t.Errorf("API-Version header = %q", got)
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, window)
		case http.MethodPut:
			raw, _ := io.ReadAll(r.Body)
			putBody = string(raw)
			writeJSON(w, 200, putBody)
		case http.MethodDelete:
			deleted = true
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.GetMaintenanceWindow(context.Background(), cuid)
	if err != nil {
		t.Fatalf("GetMaintenanceWindow: %v", err)
	}
	if !got.Enabled || len(got.DaysOfWeek) != 2 || got.StartLocal != "02:00" || got.Timezone != "Europe/Berlin" {
		t.Fatalf("unexpected window: %+v", got)
	}

	set, err := c.SetMaintenanceWindow(context.Background(), cuid, MaintenanceWindow{
		Enabled: true, DaysOfWeek: []int64{0, 6}, StartLocal: "01:00", EndLocal: "03:00", Timezone: "America/Toronto",
	})
	if err != nil {
		t.Fatalf("SetMaintenanceWindow: %v", err)
	}
	if set.StartLocal != "01:00" || len(set.DaysOfWeek) != 2 {
		t.Fatalf("unexpected set result: %+v", set)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(putBody), &sent); err != nil || sent["timezone"] != "America/Toronto" {
		t.Fatalf("unexpected put body: %s (%v)", putBody, err)
	}

	if err := c.DeleteMaintenanceWindow(context.Background(), cuid); err != nil {
		t.Fatalf("DeleteMaintenanceWindow: %v", err)
	}
	if !deleted {
		t.Fatal("delete endpoint not called")
	}
}

func TestMaintenanceWindowGetError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 404, `{"title":"not found","status":404}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).GetMaintenanceWindow(context.Background(), cuid)
	if !IsNotFound(err) {
		t.Fatalf("want 404 APIError, got %v", err)
	}
}
