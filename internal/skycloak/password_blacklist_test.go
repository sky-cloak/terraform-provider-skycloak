package skycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const blacklistJSON = `{"name":"common.txt","size":12,"sha256":"abc","status":"%s","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`

func blacklistBody(status string) string {
	return strings.Replace(blacklistJSON, "%s", status, 1)
}

func TestUploadPasswordBlacklistFileMultipart(t *testing.T) {
	var sawPath, sawName, sawApplyNow, sawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.Method + " " + r.URL.Path
		fields, _ := parseMultipart(t, r)
		sawName = strings.Join(fields["name"], "")
		sawApplyNow = strings.Join(fields["apply_now"], "")
		sawBody = strings.Join(fields["file"], "")
		writeJSON(w, http.StatusCreated, blacklistBody("waiting_for_restart"))
	}))
	defer srv.Close()

	f, err := newTestClient(srv.URL).UploadPasswordBlacklistFile(context.Background(), cuid, UploadPasswordBlacklistFileRequest{
		Name: "common.txt", ApplyNow: true, Content: []byte("hunter2\n"),
	})
	if err != nil {
		t.Fatalf("UploadPasswordBlacklistFile: %v", err)
	}
	if sawPath != "POST /clusters/"+cuid+"/password-blacklist-files" {
		t.Fatalf("request = %q", sawPath)
	}
	if sawName != "common.txt" || sawApplyNow != "true" || sawBody != "hunter2\n" {
		t.Fatalf("fields name=%q apply_now=%q file=%q", sawName, sawApplyNow, sawBody)
	}
	if f.Name != "common.txt" || f.Size != 12 || f.SHA256 != "abc" || f.Status != PasswordBlacklistWaitingForRestart {
		t.Fatalf("file = %+v", f)
	}
}

func TestUploadPasswordBlacklistFileOmitsApplyNowByDefault(t *testing.T) {
	var fields map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields, _ = parseMultipart(t, r)
		writeJSON(w, http.StatusCreated, blacklistBody("waiting_for_restart"))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).UploadPasswordBlacklistFile(context.Background(), cuid, UploadPasswordBlacklistFileRequest{Name: "a", Content: []byte("x")})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["apply_now"]; ok {
		t.Fatalf("apply_now sent without being set: %v", fields)
	}
}

func TestUploadPasswordBlacklistFileRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusConflict, `{"title":"Conflict","status":409,"detail":"name exists"}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).UploadPasswordBlacklistFile(context.Background(), cuid, UploadPasswordBlacklistFileRequest{Name: "a", Content: []byte("x")})
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("err = %v, want a 409 API error", err)
	}
}

func TestGetAndDeletePasswordBlacklistFile(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, blacklistBody("ready"))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)

	f, err := c.GetPasswordBlacklistFile(context.Background(), cuid, "common.txt")
	if err != nil || f.Status != PasswordBlacklistReady {
		t.Fatalf("Get: %+v, %v", f, err)
	}
	if err := c.DeletePasswordBlacklistFile(context.Background(), cuid, "common.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	want := []string{"GET /clusters/" + cuid + "/password-blacklist-files/common.txt", "DELETE /clusters/" + cuid + "/password-blacklist-files/common.txt"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestDeletePasswordBlacklistFileInUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusConflict, `{"title":"Conflict","status":409,"detail":"a realm policy names the file"}`)
	}))
	defer srv.Close()

	err := newTestClient(srv.URL).DeletePasswordBlacklistFile(context.Background(), cuid, "common.txt")
	if apiErr, ok := AsAPIError(err); !ok || apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("err = %v, want a 409 API error", err)
	}
}

func TestWaitForPasswordBlacklistFile(t *testing.T) {
	old := passwordBlacklistPollInterval
	passwordBlacklistPollInterval = time.Millisecond
	defer func() { passwordBlacklistPollInterval = old }()

	tests := []struct {
		name       string
		statuses   []string
		untilReady bool
		wantStatus string
		wantErr    bool
		wantGets   int
	}{
		{"ready after delivering", []string{"delivering", "ready"}, false, "ready", false, 2},
		{"waiting for restart does not block", []string{"waiting_for_restart"}, false, "waiting_for_restart", false, 1},
		{"apply_now waits through the restart", []string{"waiting_for_restart", "delivering", "ready"}, true, "ready", false, 3},
		{"failed is an error", []string{"delivering", "failed"}, false, "failed", true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gets := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				s := tt.statuses[min(gets, len(tt.statuses)-1)]
				gets++
				writeJSON(w, http.StatusOK, blacklistBody(s))
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			f, err := newTestClient(srv.URL).WaitForPasswordBlacklistFile(ctx, cuid, "common.txt", tt.untilReady)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if f == nil || f.Status != tt.wantStatus {
				t.Fatalf("file = %+v, want status %s", f, tt.wantStatus)
			}
			if gets != tt.wantGets {
				t.Fatalf("gets = %d, want %d", gets, tt.wantGets)
			}
		})
	}
}
