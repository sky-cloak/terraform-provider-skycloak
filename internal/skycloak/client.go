// Package skycloak is a minimal typed client for the Skycloak public API.
//
// Hand-written subset covering the endpoints the provider uses today. The full
// client is generated from the OpenAPI spec via oapi-codegen (internal/apiclient);
// types and method shapes here are kept compatible with that.
package skycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultEndpoint = "https://api.skycloak.io"

// Client talks to the Skycloak public API; every request is workspace-scoped by
// the API key.
type Client struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	apiVersion string
	userAgent  string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying HTTP client (useful in tests).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// New builds a Client. endpoint defaults to https://api.skycloak.io when empty.
func New(endpoint, apiKey, apiVersion string, opts ...Option) *Client {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		endpoint:   endpoint,
		apiKey:     apiKey,
		apiVersion: apiVersion,
		userAgent:  "skycloak-go/dev",
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Problem is the RFC 9457 application/problem+json error body.
type Problem struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title,omitempty"`
	Status int    `json:"status,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// APIError wraps a non-2xx response.
type APIError struct {
	StatusCode int
	Problem    Problem
}

func (e *APIError) Error() string {
	if e.Problem.Detail != "" {
		return fmt.Sprintf("skycloak api %d %s: %s", e.StatusCode, e.Problem.Title, e.Problem.Detail)
	}
	if e.Problem.Title != "" {
		return fmt.Sprintf("skycloak api %d: %s", e.StatusCode, e.Problem.Title)
	}
	return fmt.Sprintf("skycloak api: unexpected status %d", e.StatusCode)
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

// Cluster mirrors the public API Cluster resource (subset).
type Cluster struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Size      string `json:"size"`
	Version   string `json:"version"`
	Location  string `json:"location"`
	Status    string `json:"status"`
	URL       string `json:"url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// CreateClusterRequest is the body for creating a cluster.
type CreateClusterRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     string `json:"size"`
	Version  string `json:"version"`
	Location string `json:"location"`
}

// UpdateClusterRequest is the body for updating mutable cluster fields.
type UpdateClusterRequest struct {
	Size    *string `json:"size,omitempty"`
	Version *string `json:"version,omitempty"`
}

// ListClustersParams holds the query parameters for ListClusters.
type ListClustersParams struct {
	Limit  int
	Offset int
}

// ListClusters returns the workspace's clusters.
func (c *Client) ListClusters(ctx context.Context, p ListClustersParams) ([]Cluster, error) {
	q := url.Values{}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Offset > 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	var out []Cluster
	if err := c.do(ctx, http.MethodGet, "/clusters", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetCluster returns a single cluster by ID.
func (c *Client) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	var out Cluster
	if err := c.do(ctx, http.MethodGet, "/clusters/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCluster creates a cluster. The API may return 202 Accepted; the body
// carries the provisioning resource either way.
func (c *Client) CreateCluster(ctx context.Context, req CreateClusterRequest) (*Cluster, error) {
	var out Cluster
	if err := c.do(ctx, http.MethodPost, "/clusters", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateCluster patches mutable fields of a cluster.
func (c *Client) UpdateCluster(ctx context.Context, id string, req UpdateClusterRequest) (*Cluster, error) {
	var out Cluster
	if err := c.do(ctx, http.MethodPatch, "/clusters/"+url.PathEscape(id), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteCluster deletes a cluster (async on the server side).
func (c *Client) DeleteCluster(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/clusters/"+url.PathEscape(id), nil, nil, nil)
}

// WaitForClusterReady polls the cluster until it reaches "available", returns a
// terminal-failure error if it reaches "failed", or surfaces the context error
// on timeout/cancel.
func (c *Client) WaitForClusterReady(ctx context.Context, id string) (*Cluster, error) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		cl, err := c.GetCluster(ctx, id)
		if err != nil {
			return nil, err
		}
		switch cl.Status {
		case "available":
			return cl, nil
		case "failed":
			return cl, fmt.Errorf("cluster %s entered failed state", id)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Realm mirrors the public API Realm resource (subset). Realms are keyed by
// Name within a cluster.
type Realm struct {
	Name                        string `json:"name"`
	DisplayName                 string `json:"display_name,omitempty"`
	Enabled                     bool   `json:"enabled"`
	SSLRequired                 string `json:"ssl_required,omitempty"`
	RegistrationAllowed         bool   `json:"registration_allowed"`
	RegistrationEmailAsUsername bool   `json:"registration_email_as_username"`
	LoginWithEmailAllowed       bool   `json:"login_with_email_allowed"`
	DuplicateEmailsAllowed      bool   `json:"duplicate_emails_allowed"`
	CreatedAt                   string `json:"created_at,omitempty"`
}

// ListRealms returns the realms in a cluster.
func (c *Client) ListRealms(ctx context.Context, clusterID string) ([]Realm, error) {
	var out []Realm
	if err := c.do(ctx, http.MethodGet, "/clusters/"+url.PathEscape(clusterID)+"/realms", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetRealm returns a single realm by name.
func (c *Client) GetRealm(ctx context.Context, clusterID, name string) (*Realm, error) {
	var out Realm
	if err := c.do(ctx, http.MethodGet, realmPath(clusterID, name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateRealm creates a realm in a cluster.
func (c *Client) CreateRealm(ctx context.Context, clusterID string, r Realm) (*Realm, error) {
	var out Realm
	if err := c.do(ctx, http.MethodPost, "/clusters/"+url.PathEscape(clusterID)+"/realms", nil, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateRealm updates a realm's mutable fields.
func (c *Client) UpdateRealm(ctx context.Context, clusterID, name string, r Realm) (*Realm, error) {
	var out Realm
	if err := c.do(ctx, http.MethodPut, realmPath(clusterID, name), nil, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRealm deletes a realm.
func (c *Client) DeleteRealm(ctx context.Context, clusterID, name string) error {
	return c.do(ctx, http.MethodDelete, realmPath(clusterID, name), nil, nil, nil)
}

func realmPath(clusterID, name string) string {
	return "/clusters/" + url.PathEscape(clusterID) + "/realms/" + url.PathEscape(name)
}

// Application mirrors the public API Application (OIDC/SAML client) resource.
// Keyed by ClientID within a realm. ClientSecret is only returned on create and
// rotate-secret.
type Application struct {
	ClientID              string   `json:"client_id"`
	Name                  string   `json:"name,omitempty"`
	Description           string   `json:"description,omitempty"`
	Type                  string   `json:"type,omitempty"`
	Protocol              string   `json:"protocol,omitempty"`
	Status                string   `json:"status,omitempty"`
	RedirectURIs          []string `json:"redirect_uris,omitempty"`
	GrantTypes            []string `json:"grant_types,omitempty"`
	PKCERequired          bool     `json:"pkce_required"`
	ConsentRequired       bool     `json:"consent_required"`
	ServiceAccountEnabled bool     `json:"service_account_enabled"`
	ClientSecret          string   `json:"client_secret,omitempty"`
	CreatedAt             string   `json:"created_at,omitempty"`
	UpdatedAt             string   `json:"updated_at,omitempty"`
}

// ListApplications returns the applications in a realm.
func (c *Client) ListApplications(ctx context.Context, clusterID, realm string) ([]Application, error) {
	var out []Application
	if err := c.do(ctx, http.MethodGet, appsPath(clusterID, realm), nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetApplication returns a single application by client ID.
func (c *Client) GetApplication(ctx context.Context, clusterID, realm, clientID string) (*Application, error) {
	var out Application
	if err := c.do(ctx, http.MethodGet, appPath(clusterID, realm, clientID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateApplication creates an application in a realm.
func (c *Client) CreateApplication(ctx context.Context, clusterID, realm string, a Application) (*Application, error) {
	var out Application
	if err := c.do(ctx, http.MethodPost, appsPath(clusterID, realm), nil, a, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateApplication updates an application's mutable fields.
func (c *Client) UpdateApplication(ctx context.Context, clusterID, realm, clientID string, a Application) (*Application, error) {
	var out Application
	if err := c.do(ctx, http.MethodPut, appPath(clusterID, realm, clientID), nil, a, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteApplication deletes an application.
func (c *Client) DeleteApplication(ctx context.Context, clusterID, realm, clientID string) error {
	return c.do(ctx, http.MethodDelete, appPath(clusterID, realm, clientID), nil, nil, nil)
}

func appsPath(clusterID, realm string) string {
	return "/clusters/" + url.PathEscape(clusterID) + "/realms/" + url.PathEscape(realm) + "/applications"
}

func appPath(clusterID, realm, clientID string) string {
	return appsPath(clusterID, realm) + "/" + url.PathEscape(clientID)
}

// IdentityProvider mirrors the public API identity-provider resource. Keyed by
// ProviderID (the unique alias) within a realm. Providers with
// ExternallyManaged=true reject updates/deletes (403).
type IdentityProvider struct {
	ProviderID        string            `json:"provider_id"`
	Type              string            `json:"type,omitempty"`
	DisplayName       string            `json:"display_name,omitempty"`
	Enabled           bool              `json:"enabled"`
	ClientID          string            `json:"client_id,omitempty"`
	Config            map[string]string `json:"config,omitempty"`
	ExternallyManaged bool              `json:"externally_managed,omitempty"`
	CreatedAt         string            `json:"created_at,omitempty"`
	UpdatedAt         string            `json:"updated_at,omitempty"`
}

// ListIdentityProviders returns the identity providers in a realm.
func (c *Client) ListIdentityProviders(ctx context.Context, clusterID, realm string) ([]IdentityProvider, error) {
	var out []IdentityProvider
	if err := c.do(ctx, http.MethodGet, idpsPath(clusterID, realm), nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetIdentityProvider returns a single identity provider by its provider ID.
func (c *Client) GetIdentityProvider(ctx context.Context, clusterID, realm, providerID string) (*IdentityProvider, error) {
	var out IdentityProvider
	if err := c.do(ctx, http.MethodGet, idpPath(clusterID, realm, providerID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateIdentityProvider creates an identity provider in a realm.
func (c *Client) CreateIdentityProvider(ctx context.Context, clusterID, realm string, idp IdentityProvider) (*IdentityProvider, error) {
	var out IdentityProvider
	if err := c.do(ctx, http.MethodPost, idpsPath(clusterID, realm), nil, idp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateIdentityProvider updates an identity provider's mutable fields.
func (c *Client) UpdateIdentityProvider(ctx context.Context, clusterID, realm, providerID string, idp IdentityProvider) (*IdentityProvider, error) {
	var out IdentityProvider
	if err := c.do(ctx, http.MethodPut, idpPath(clusterID, realm, providerID), nil, idp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteIdentityProvider deletes an identity provider.
func (c *Client) DeleteIdentityProvider(ctx context.Context, clusterID, realm, providerID string) error {
	return c.do(ctx, http.MethodDelete, idpPath(clusterID, realm, providerID), nil, nil, nil)
}

func idpsPath(clusterID, realm string) string {
	return "/clusters/" + url.PathEscape(clusterID) + "/realms/" + url.PathEscape(realm) + "/identity-providers"
}

func idpPath(clusterID, realm, providerID string) string {
	return idpsPath(clusterID, realm) + "/" + url.PathEscape(providerID)
}

// SMTPConfig mirrors the public API SmtpConfig (read) resource. Secrets are
// never returned: HasPassword / HasClientSecret report whether one is stored.
type SMTPConfig struct {
	Host            string `json:"host"`
	Port            int64  `json:"port"`
	Encryption      string `json:"encryption,omitempty"`
	FromEmail       string `json:"from_email"`
	FromName        string `json:"from_name,omitempty"`
	AuthType        string `json:"auth_type"`
	Username        string `json:"username,omitempty"`
	HasPassword     bool   `json:"has_password"`
	TokenURL        string `json:"token_url,omitempty"`
	TokenScope      string `json:"token_scope,omitempty"`
	ClientID        string `json:"client_id,omitempty"`
	HasClientSecret bool   `json:"has_client_secret"`
	Status          string `json:"status,omitempty"`
}

// UpsertSMTPRequest is the body for creating/updating SMTP config (PUT upsert).
// Omit Password/ClientSecret to retain the stored value.
type UpsertSMTPRequest struct {
	Host         string `json:"host"`
	Port         int64  `json:"port"`
	Encryption   string `json:"encryption,omitempty"`
	FromEmail    string `json:"from_email"`
	FromName     string `json:"from_name,omitempty"`
	AuthType     string `json:"auth_type"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	TokenURL     string `json:"token_url,omitempty"`
	TokenScope   string `json:"token_scope,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// GetSMTP returns the SMTP configuration for a realm.
func (c *Client) GetSMTP(ctx context.Context, clusterID, realm string) (*SMTPConfig, error) {
	var out SMTPConfig
	if err := c.do(ctx, http.MethodGet, smtpPath(clusterID, realm), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpsertSMTP creates or updates the SMTP configuration for a realm.
func (c *Client) UpsertSMTP(ctx context.Context, clusterID, realm string, req UpsertSMTPRequest) (*SMTPConfig, error) {
	var out SMTPConfig
	if err := c.do(ctx, http.MethodPut, smtpPath(clusterID, realm), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSMTP removes the SMTP configuration for a realm.
func (c *Client) DeleteSMTP(ctx context.Context, clusterID, realm string) error {
	return c.do(ctx, http.MethodDelete, smtpPath(clusterID, realm), nil, nil, nil)
}

func smtpPath(clusterID, realm string) string {
	return "/clusters/" + url.PathEscape(clusterID) + "/realms/" + url.PathEscape(realm) + "/smtp"
}

// ClusterLocationInfo is a supported deployment region (metadata).
type ClusterLocationInfo struct {
	Location  string `json:"location"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

// ClusterTypeInfo is a supported cluster type (metadata).
type ClusterTypeInfo struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

// ClusterFeatureInfo is a Keycloak feature flag available to tenant clusters.
type ClusterFeatureInfo struct {
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name"`
	Description *string `json:"description"`
	Preview     bool    `json:"preview"`
	MinVersion  *string `json:"min_version"`
	MaxVersion  *string `json:"max_version"`
}

// ListClusterLocations returns the supported deployment regions.
func (c *Client) ListClusterLocations(ctx context.Context) ([]ClusterLocationInfo, error) {
	var out []ClusterLocationInfo
	if err := c.do(ctx, http.MethodGet, "/cluster-locations", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListClusterTypes returns the supported cluster types.
func (c *Client) ListClusterTypes(ctx context.Context) ([]ClusterTypeInfo, error) {
	var out []ClusterTypeInfo
	if err := c.do(ctx, http.MethodGet, "/cluster-types", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListClusterFeatures returns the available Keycloak feature flags.
func (c *Client) ListClusterFeatures(ctx context.Context) ([]ClusterFeatureInfo, error) {
	var out []ClusterFeatureInfo
	if err := c.do(ctx, http.MethodGet, "/cluster-features", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.endpoint + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiVersion != "" {
		req.Header.Set("API-Version", c.apiVersion)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		_ = json.Unmarshal(data, &apiErr.Problem)
		return apiErr
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}
