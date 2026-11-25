package skycloak

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"time"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// Password blacklist file statuses reported by the API.
const (
	PasswordBlacklistWaitingForRestart = "waiting_for_restart"
	PasswordBlacklistDelivering        = "delivering"
	PasswordBlacklistReady             = "ready"
	PasswordBlacklistFailed            = "failed"
)

// passwordBlacklistPollInterval is how often a file's status is re-read while
// waiting for it. It is a variable so tests do not have to sit through it.
var passwordBlacklistPollInterval = 2 * time.Second

// PasswordBlacklistFile mirrors the public API password blacklist file. Files
// cannot be changed after upload, so there is no update call.
type PasswordBlacklistFile struct {
	Name                 string
	Size                 int64
	SHA256               string
	Status               string
	CreatedAt, UpdatedAt string
}

// UploadPasswordBlacklistFileRequest is the body for uploading a file.
type UploadPasswordBlacklistFileRequest struct {
	Name string
	// ApplyNow runs the restart that adds the blacklist folder at once instead
	// of in the maintenance window.
	ApplyNow bool
	Content  []byte
}

func passwordBlacklistFileFromAPI(f *apiclient.PasswordBlacklistFile) *PasswordBlacklistFile {
	return &PasswordBlacklistFile{
		Name: f.Name, Size: f.Size, SHA256: f.Sha256, Status: string(f.Status),
		CreatedAt: fmtTime(f.CreatedAt), UpdatedAt: fmtTime(f.UpdatedAt),
	}
}

// UploadPasswordBlacklistFile uploads a password blacklist file to a dedicated
// cluster.
func (c *Client) UploadPasswordBlacklistFile(ctx context.Context, clusterID string, req UploadPasswordBlacklistFileRequest) (*PasswordBlacklistFile, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", req.Name)
	if req.ApplyNow {
		_ = mw.WriteField("apply_now", "true")
	}
	if err := filePart(mw, "file", req.Name, "text/plain", req.Content); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	resp, err := c.gen.UploadPasswordBlacklistFileWithBodyWithResponse(ctx, cid(clusterID), &apiclient.UploadPasswordBlacklistFileParams{APIVersion: c.ver()}, mw.FormDataContentType(), &buf)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return passwordBlacklistFileFromAPI(resp.JSON201), nil
}

// GetPasswordBlacklistFile returns one file by name.
func (c *Client) GetPasswordBlacklistFile(ctx context.Context, clusterID, name string) (*PasswordBlacklistFile, error) {
	resp, err := c.gen.GetPasswordBlacklistFileWithResponse(ctx, cid(clusterID), name, &apiclient.GetPasswordBlacklistFileParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return passwordBlacklistFileFromAPI(resp.JSON200), nil
}

// DeletePasswordBlacklistFile removes a file from storage and from the
// cluster. The API refuses with 409 while a realm password policy names it.
func (c *Client) DeletePasswordBlacklistFile(ctx context.Context, clusterID, name string) error {
	resp, err := c.gen.DeletePasswordBlacklistFileWithResponse(ctx, cid(clusterID), name, &apiclient.DeletePasswordBlacklistFileParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// WaitForPasswordBlacklistFile polls a file until it is ready, or until it
// leaves "delivering" when untilReady is false. A file waiting for the
// maintenance window can stay in that state for a long time, so callers that
// did not ask for an immediate restart stop waiting once delivery is not in
// progress. A failed file is returned together with an error.
func (c *Client) WaitForPasswordBlacklistFile(ctx context.Context, clusterID, name string, untilReady bool) (*PasswordBlacklistFile, error) {
	ticker := time.NewTicker(passwordBlacklistPollInterval)
	defer ticker.Stop()
	for {
		f, err := c.GetPasswordBlacklistFile(ctx, clusterID, name)
		if err != nil {
			return nil, err
		}
		switch f.Status {
		case PasswordBlacklistReady:
			return f, nil
		case PasswordBlacklistFailed:
			return f, fmt.Errorf("password blacklist file %s failed to reach the cluster", name)
		case PasswordBlacklistWaitingForRestart:
			if !untilReady {
				return f, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
