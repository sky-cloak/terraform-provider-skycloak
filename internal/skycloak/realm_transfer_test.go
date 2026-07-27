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

const (
	realmExportUID = "77777777-7777-7777-7777-777777777777"
	realmImportUID = "88888888-8888-8888-8888-888888888888"
)

func TestRealmExportLifecycle(t *testing.T) {
	processing := `{"id":"` + realmExportUID + `","cluster_id":"` + cuid + `","realm":"customer-portal","scope":"full",` +
		`"status":"processing","progress":25,"source_version":null,"sha256_checksum":null,"download_url":null,` +
		`"error_message":null,"created_at":"2026-01-01T00:00:00Z","completed_at":null,"expires_at":null}`
	done := `{"id":"` + realmExportUID + `","cluster_id":"` + cuid + `","realm":"customer-portal","scope":"full",` +
		`"status":"completed","progress":100,"source_version":"26.1","sha256_checksum":"abc123","download_url":"https://dl/realm.zip.enc",` +
		`"error_message":null,"created_at":"2026-01-01T00:00:00Z","completed_at":"2026-01-01T00:02:00Z","expires_at":"2026-01-02T00:02:00Z"}`

	var gotBody map[string]any
	var gets int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/clusters/"+cuid+"/realms/customer-portal/exports":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			writeJSON(w, http.StatusAccepted, processing)
		case r.Method == http.MethodGet && r.URL.Path == "/realm-exports/"+realmExportUID:
			gets++
			if gets >= 2 {
				writeJSON(w, 200, done)
				return
			}
			writeJSON(w, 200, processing)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	exp, err := c.CreateRealmExport(context.Background(), cuid, "customer-portal",
		CreateRealmExportRequest{Scope: "full", EncryptionPassword: "a-strong-passphrase"})
	if err != nil || exp.Status != "processing" || exp.Realm != "customer-portal" || exp.Progress != 25 {
		t.Fatalf("CreateRealmExport: %+v, %v", exp, err)
	}
	if gotBody["encryption_password"] != "a-strong-passphrase" || gotBody["scope"] != "full" {
		t.Fatalf("request body = %v, want the password and scope forwarded", gotBody)
	}

	final, err := c.WaitForRealmExport(context.Background(), realmExportUID)
	if err != nil || final.Status != "completed" || final.DownloadURL == "" || final.SourceVersion != "26.1" {
		t.Fatalf("WaitForRealmExport: %+v, %v", final, err)
	}
}

func TestWaitForRealmExportFails(t *testing.T) {
	failed := `{"id":"` + realmExportUID + `","cluster_id":"` + cuid + `","realm":"customer-portal","scope":"full",` +
		`"status":"failed","progress":0,"source_version":null,"sha256_checksum":null,"download_url":null,` +
		`"error_message":"kc.sh export crashed","created_at":"2026-01-01T00:00:00Z","completed_at":null,"expires_at":null}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, failed)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).WaitForRealmExport(context.Background(), realmExportUID)
	if err == nil || !strings.Contains(err.Error(), "kc.sh export crashed") {
		t.Fatalf("expected the failure message to surface, got %v", err)
	}
}

// TestRealmImportUploadFlow covers the three-step upload path: presign, PUT the
// artifact to storage, then start the import with the returned key.
func TestRealmImportUploadFlow(t *testing.T) {
	processing := `{"id":"` + realmImportUID + `","cluster_id":"` + cuid + `","realm":"customer-portal","source_kind":"upload",` +
		`"status":"processing","progress":10,"source_version":"26.1","target_version":"26.1","error_message":null,` +
		`"created_at":"2026-01-01T00:00:00Z","completed_at":null}`
	done := `{"id":"` + realmImportUID + `","cluster_id":"` + cuid + `","realm":"customer-portal","source_kind":"upload",` +
		`"status":"completed","progress":100,"source_version":"26.1","target_version":"26.1","error_message":null,` +
		`"created_at":"2026-01-01T00:00:00Z","completed_at":"2026-01-01T00:03:00Z"}`

	var uploaded []byte
	var uploadAuth, uploadAPIKey string
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "want PUT", http.StatusMethodNotAllowed)
			return
		}
		uploadAuth = r.Header.Get("Authorization")
		uploadAPIKey = r.Header.Get("API-Key")
		uploaded, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer storage.Close()

	var importBody map[string]any
	var gets int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/clusters/"+cuid+"/realms/import/upload-url":
			writeJSON(w, 200, `{"upload_url":"`+storage.URL+`/put","s3_key":"realm-imports/uploads/ws/abc.zip.enc"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/clusters/"+cuid+"/realms/import":
			_ = json.NewDecoder(r.Body).Decode(&importBody)
			writeJSON(w, http.StatusAccepted, processing)
		case r.Method == http.MethodGet && r.URL.Path == "/realm-imports/"+realmImportUID:
			gets++
			if gets >= 2 {
				writeJSON(w, 200, done)
				return
			}
			writeJSON(w, 200, processing)
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	c := newTestClient(api.URL)
	presigned, err := c.PresignRealmImportUpload(context.Background(), cuid)
	if err != nil || presigned.S3Key != "realm-imports/uploads/ws/abc.zip.enc" {
		t.Fatalf("PresignRealmImportUpload: %+v, %v", presigned, err)
	}

	if err := c.UploadRealmImportArtifact(context.Background(), presigned.UploadURL, []byte("realm-bytes")); err != nil {
		t.Fatalf("UploadRealmImportArtifact: %v", err)
	}
	if string(uploaded) != "realm-bytes" {
		t.Fatalf("uploaded = %q, want realm-bytes", uploaded)
	}
	// The presigned URL authenticates itself; leaking the Skycloak API key to
	// object storage would be a credential disclosure.
	if uploadAPIKey != "" || uploadAuth != "" {
		t.Fatalf("upload leaked credentials: API-Key=%q Authorization=%q", uploadAPIKey, uploadAuth)
	}

	imp, err := c.CreateRealmImport(context.Background(), cuid, CreateRealmImportRequest{
		SourceKind: "upload", UploadS3Key: presigned.S3Key, Password: "a-strong-passphrase",
	})
	if err != nil || imp.Status != "processing" {
		t.Fatalf("CreateRealmImport: %+v, %v", imp, err)
	}
	if importBody["upload_s3_key"] != presigned.S3Key || importBody["source_kind"] != "upload" {
		t.Fatalf("import body = %v, want the s3 key and source kind forwarded", importBody)
	}
	if _, ok := importBody["source_export_id"]; ok {
		t.Fatalf("import body = %v, want source_export_id omitted on an upload import", importBody)
	}

	final, err := c.WaitForRealmImport(context.Background(), realmImportUID)
	if err != nil || final.Status != "completed" || final.Realm != "customer-portal" {
		t.Fatalf("WaitForRealmImport: %+v, %v", final, err)
	}
}

func TestRealmImportStoredSource(t *testing.T) {
	accepted := `{"id":"` + realmImportUID + `","cluster_id":"` + cuid + `","realm":"customer-portal","source_kind":"stored",` +
		`"status":"pending","progress":0,"source_version":null,"target_version":null,"error_message":null,` +
		`"created_at":"2026-01-01T00:00:00Z","completed_at":null}`
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeJSON(w, http.StatusAccepted, accepted)
	}))
	defer srv.Close()

	imp, err := newTestClient(srv.URL).CreateRealmImport(context.Background(), cuid, CreateRealmImportRequest{
		SourceKind: "stored", SourceExportID: realmExportUID, Password: "pw",
	})
	if err != nil || imp.SourceKind != "stored" {
		t.Fatalf("CreateRealmImport: %+v, %v", imp, err)
	}
	if body["source_export_id"] != realmExportUID {
		t.Fatalf("body = %v, want source_export_id forwarded", body)
	}
	if _, ok := body["upload_s3_key"]; ok {
		t.Fatalf("body = %v, want upload_s3_key omitted on a stored import", body)
	}
}

func TestUploadRealmImportArtifactSurfacesStorageError(t *testing.T) {
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "AccessDenied", http.StatusForbidden)
	}))
	defer storage.Close()

	err := newTestClient("http://unused").UploadRealmImportArtifact(context.Background(), storage.URL, []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected the storage status to surface, got %v", err)
	}
}
