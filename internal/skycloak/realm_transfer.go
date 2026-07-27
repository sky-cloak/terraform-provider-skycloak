package skycloak

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// Realm export/import. Both are asynchronous jobs: the create call returns 202
// with the job resource, and the job is polled until it reaches a terminal
// state. Unlike most resources these are addressed workspace-wide once created
// (/realm-exports/{id}, /realm-imports/{id}), so reads take no cluster ID.

// ---- Realm export ----

// RealmExport is a single-realm export job. Every realm export is encrypted,
// because it always contains credentials.
type RealmExport struct {
	ID             string
	ClusterID      string
	Realm          string
	Scope          string
	Status         string
	Progress       int64
	SourceVersion  string
	Sha256Checksum string
	DownloadURL    string
	ErrorMessage   string
	CreatedAt      string
	CompletedAt    string
	ExpiresAt      string
}

func realmExportFromAPI(e *apiclient.RealmExport) *RealmExport {
	return &RealmExport{
		ID: e.Id.String(), ClusterID: e.ClusterId.String(), Realm: e.Realm, Scope: string(e.Scope),
		Status: string(e.Status), Progress: int64(e.Progress), SourceVersion: nStr(e.SourceVersion),
		Sha256Checksum: nStr(e.Sha256Checksum), DownloadURL: nStr(e.DownloadUrl), ErrorMessage: nStr(e.ErrorMessage),
		CreatedAt: fmtTime(e.CreatedAt), CompletedAt: nTime(e.CompletedAt), ExpiresAt: nTime(e.ExpiresAt),
	}
}

// CreateRealmExportRequest is the body for starting a realm export. The
// password is mandatory: the API rejects an unencrypted realm export.
type CreateRealmExportRequest struct {
	Scope              string
	EncryptionPassword string
}

// CreateRealmExport queues an encrypted export of a single realm (async, 202).
func (c *Client) CreateRealmExport(ctx context.Context, clusterID, realm string, req CreateRealmExportRequest) (*RealmExport, error) {
	body := apiclient.CreateRealmExportJSONRequestBody{EncryptionPassword: req.EncryptionPassword}
	if req.Scope != "" {
		s := apiclient.RealmExportScope(req.Scope)
		body.Scope = &s
	}
	resp, err := c.gen.CreateRealmExportWithResponse(ctx, cid(clusterID), realm, &apiclient.CreateRealmExportParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON202 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return realmExportFromAPI(resp.JSON202), nil
}

// GetRealmExport returns a realm export by ID.
func (c *Client) GetRealmExport(ctx context.Context, exportID string) (*RealmExport, error) {
	resp, err := c.gen.GetRealmExportWithResponse(ctx, uid(exportID), &apiclient.GetRealmExportParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return realmExportFromAPI(resp.JSON200), nil
}

// WaitForRealmExport polls a realm export until it completes, fails, or the
// context is cancelled.
func (c *Client) WaitForRealmExport(ctx context.Context, exportID string) (*RealmExport, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		e, err := c.GetRealmExport(ctx, exportID)
		if err != nil {
			return nil, err
		}
		switch e.Status {
		case "completed":
			return e, nil
		case "failed":
			return e, fmt.Errorf("realm export %s failed: %s", exportID, e.ErrorMessage)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// ---- Realm import ----

// RealmImport is a realm import job.
type RealmImport struct {
	ID            string
	ClusterID     string
	Realm         string
	SourceKind    string
	Status        string
	Progress      int64
	SourceVersion string
	TargetVersion string
	ErrorMessage  string
	CreatedAt     string
	CompletedAt   string
}

func realmImportFromAPI(i *apiclient.RealmImport) *RealmImport {
	return &RealmImport{
		ID: i.Id.String(), ClusterID: i.ClusterId.String(), Realm: i.Realm, SourceKind: string(i.SourceKind),
		Status: string(i.Status), Progress: int64(i.Progress), SourceVersion: nStr(i.SourceVersion),
		TargetVersion: nStr(i.TargetVersion), ErrorMessage: nStr(i.ErrorMessage),
		CreatedAt: fmtTime(i.CreatedAt), CompletedAt: nTime(i.CompletedAt),
	}
}

// PresignedUpload is a short-lived PUT target for an import artifact.
type PresignedUpload struct {
	UploadURL string
	S3Key     string
}

// PresignRealmImportUpload returns a presigned URL to upload an import artifact
// to, plus the object key to hand back to CreateRealmImport.
func (c *Client) PresignRealmImportUpload(ctx context.Context, clusterID string) (*PresignedUpload, error) {
	resp, err := c.gen.PresignRealmImportUploadWithResponse(ctx, cid(clusterID), &apiclient.PresignRealmImportUploadParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return &PresignedUpload{UploadURL: resp.JSON200.UploadUrl, S3Key: resp.JSON200.S3Key}, nil
}

// UploadRealmImportArtifact PUTs the artifact to a presigned URL. The URL
// carries its own credentials, so this goes out through the bare HTTP client and
// deliberately sends no API key to object storage.
func (c *Client) UploadRealmImportArtifact(ctx context.Context, uploadURL string, content []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(content))

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("uploading realm import artifact: storage returned %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}

// CreateRealmImportRequest is the body for starting a realm import. Set either
// UploadS3Key (source kind "upload") or SourceExportID (source kind "stored").
type CreateRealmImportRequest struct {
	SourceKind     string
	UploadS3Key    string
	SourceExportID string
	Password       string
}

// CreateRealmImport starts a realm import (async, 202).
func (c *Client) CreateRealmImport(ctx context.Context, clusterID string, req CreateRealmImportRequest) (*RealmImport, error) {
	body := apiclient.CreateRealmImportJSONRequestBody{
		UploadS3Key:    strPtr(req.UploadS3Key),
		SourceExportId: strPtr(req.SourceExportID),
		Password:       strPtr(req.Password),
	}
	if req.SourceKind != "" {
		sk := apiclient.RealmImportSourceKind(req.SourceKind)
		body.SourceKind = &sk
	}
	resp, err := c.gen.CreateRealmImportWithResponse(ctx, cid(clusterID), &apiclient.CreateRealmImportParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON202 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return realmImportFromAPI(resp.JSON202), nil
}

// GetRealmImport returns a realm import by ID.
func (c *Client) GetRealmImport(ctx context.Context, importID string) (*RealmImport, error) {
	resp, err := c.gen.GetRealmImportWithResponse(ctx, uid(importID), &apiclient.GetRealmImportParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return realmImportFromAPI(resp.JSON200), nil
}

// WaitForRealmImport polls a realm import until it completes, fails, or the
// context is cancelled.
func (c *Client) WaitForRealmImport(ctx context.Context, importID string) (*RealmImport, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		i, err := c.GetRealmImport(ctx, importID)
		if err != nil {
			return nil, err
		}
		switch i.Status {
		case "completed":
			return i, nil
		case "failed":
			return i, fmt.Errorf("realm import %s failed: %s", importID, i.ErrorMessage)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
