package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const exportUID = "66666666-6666-6666-6666-666666666666"

func TestExportLifecycle(t *testing.T) {
	pending := `{"id":"` + exportUID + `","cluster_id":"` + cuid + `","format":"pgdump","status":"processing","progress":40,` +
		`"include_credentials":false,"is_encrypted":false,"file_size_bytes":null,"sha256_checksum":null,"download_url":null,` +
		`"error_message":null,"created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:00:01Z","completed_at":null,"expires_at":null}`
	done := `{"id":"` + exportUID + `","cluster_id":"` + cuid + `","format":"pgdump","status":"completed","progress":100,` +
		`"include_credentials":false,"is_encrypted":false,"file_size_bytes":1024,"sha256_checksum":"abc123","download_url":"https://dl/export.zip",` +
		`"error_message":null,"created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:00:01Z","completed_at":"2026-01-01T00:01:00Z","expires_at":"2026-01-02T00:00:00Z"}`
	base := "/clusters/" + cuid + "/exports"
	var gets int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			writeJSON(w, http.StatusAccepted, pending)
		case r.Method == http.MethodGet && r.URL.Path == base+"/"+exportUID:
			gets++
			if gets >= 2 {
				writeJSON(w, 200, done)
				return
			}
			writeJSON(w, 200, pending)
		case r.Method == http.MethodGet && r.URL.Path == base:
			writeJSON(w, 200, "["+done+"]")
		case r.Method == http.MethodDelete && r.URL.Path == base+"/"+exportUID:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	exp, err := c.CreateExport(context.Background(), cuid, CreateExportRequest{Format: "pgdump"})
	if err != nil || exp.Status != "processing" || exp.Progress != 40 {
		t.Fatalf("CreateExport: %+v, %v", exp, err)
	}
	final, err := c.WaitForExport(context.Background(), cuid, exportUID)
	if err != nil || final.Status != "completed" || final.DownloadURL == "" || final.FileSizeBytes != 1024 {
		t.Fatalf("WaitForExport: %+v, %v", final, err)
	}
	list, err := c.ListExports(context.Background(), cuid)
	if err != nil || len(list) != 1 || list[0].Status != "completed" {
		t.Fatalf("ListExports: %+v, %v", list, err)
	}
	if err := c.DeleteExport(context.Background(), cuid, exportUID); err != nil {
		t.Fatalf("DeleteExport: %v", err)
	}
}

func TestWaitForExportFails(t *testing.T) {
	failed := `{"id":"` + exportUID + `","cluster_id":"` + cuid + `","format":"sql","status":"failed","progress":0,` +
		`"include_credentials":false,"is_encrypted":false,"file_size_bytes":null,"sha256_checksum":null,"download_url":null,` +
		`"error_message":"disk full","created_at":"2026-01-01T00:00:00Z","started_at":null,"completed_at":null,"expires_at":null}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, failed)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).WaitForExport(context.Background(), cuid, exportUID)
	if err == nil {
		t.Fatalf("expected a failure error for a failed export")
	}
}
