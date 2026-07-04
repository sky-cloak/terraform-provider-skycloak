// Package skycloak is a thin, ergonomic facade over the generated Skycloak API
// client (internal/apiclient). It exposes domain structs and methods the
// provider's resources consume, mapping them to/from the generated wire types.
// The HTTP layer and wire types are generated from the OpenAPI spec, so they
// stay in sync with the API on `make generate`.
package skycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

const defaultEndpoint = "https://api.skycloak.io"

// Client wraps the generated API client.
type Client struct {
	gen        *apiclient.ClientWithResponses
	apiVersion string
}

// ver returns the pinned API version for the generated per-operation params
// structs. The request editor also sets the header, so the two always agree;
// an unpinned client carries DefaultAPIVersion.
func (c *Client) ver() apiclient.CommonParameters {
	return apiclient.CommonParameters(c.apiVersion)
}

// Option configures a Client.
type Option func(*config)

type config struct {
	httpClient *http.Client
	userAgent  string
}

// WithHTTPClient overrides the underlying HTTP client (useful in tests).
func WithHTTPClient(h *http.Client) Option { return func(c *config) { c.httpClient = h } }

// WithUserAgent sets the User-Agent header sent on every request.
func WithUserAgent(ua string) Option { return func(c *config) { c.userAgent = ua } }

// New builds a Client. endpoint defaults to https://api.skycloak.io when empty.
func New(endpoint, apiKey, apiVersion string, opts ...Option) *Client {
	cfg := &config{httpClient: &http.Client{Timeout: 30 * time.Second}, userAgent: "skycloak-go/dev"}
	for _, o := range opts {
		o(cfg)
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	// Retry 429/5xx with Retry-After-aware backoff.
	cfg.httpClient.Transport = &retryTransport{base: cfg.httpClient.Transport, maxRetries: 4}

	editor := func(_ context.Context, req *http.Request) error {
		req.Header.Set("API-Key", apiKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("API-Version", apiVersion)
		if cfg.userAgent != "" {
			req.Header.Set("User-Agent", cfg.userAgent)
		}
		return nil
	}

	gen, err := apiclient.NewClientWithResponses(endpoint,
		apiclient.WithHTTPClient(cfg.httpClient),
		apiclient.WithRequestEditorFn(editor),
	)
	if err != nil {
		// NewClientWithResponses only errors on an unparseable server URL.
		panic(fmt.Sprintf("skycloak: invalid endpoint %q: %v", endpoint, err))
	}
	return &Client{gen: gen, apiVersion: apiVersion}
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

// AsAPIError returns the underlying *APIError if err is (or wraps) one.
func AsAPIError(err error) (*APIError, bool) {
	var e *APIError
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// statusError builds an *APIError from an HTTP response + raw body. Used when a
// generated call returns no typed success payload.
func statusError(resp *http.Response, body []byte) error {
	code := 0
	if resp != nil {
		code = resp.StatusCode
	}
	e := &APIError{StatusCode: code}
	_ = json.Unmarshal(body, &e.Problem)
	return e
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// cid parses a cluster ID string into the generated UUID type. An invalid ID
// yields the zero UUID, which the API resolves to a 404.
func cid(s string) apiclient.ClusterId {
	id, _ := uuid.Parse(s)
	return id
}

// ---- Clusters ----

// Cluster mirrors the public API Cluster resource (subset the provider uses).
type Cluster struct {
	ID                 string
	Name               string
	Type               string
	Size               string
	Version            string
	Location           string
	Status             string
	URL                string
	AutoUpgradeEnabled bool
	CreatedAt          string
	UpdatedAt          string
}

// CreateClusterRequest is the body for creating a cluster.
type CreateClusterRequest struct {
	Name               string
	Type               string
	Size               string
	Version            string
	Location           string
	AutoUpgradeEnabled *bool
}

// UpdateClusterRequest holds the mutable cluster fields.
type UpdateClusterRequest struct {
	Size               *string
	Version            *string
	AutoUpgradeEnabled *bool
}

func boolDeref(p *bool) bool {
	return p != nil && *p
}

func clusterFromAPI(c *apiclient.Cluster) *Cluster {
	return &Cluster{
		ID: c.Id.String(), Name: string(c.Name), Type: string(c.Type), Size: string(c.Size),
		Version: string(c.Version), Location: string(c.Location), Status: string(c.Status),
		URL: c.Url, AutoUpgradeEnabled: boolDeref(c.AutoUpgradeEnabled),
		CreatedAt: fmtTime(c.CreatedAt), UpdatedAt: fmtTime(c.UpdatedAt),
	}
}

// ListClusters returns the workspace's clusters.
func (c *Client) ListClusters(ctx context.Context) ([]Cluster, error) {
	resp, err := c.gen.ListClustersWithResponse(ctx, &apiclient.ListClustersParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]Cluster, 0, len(*resp.JSON200))
	for _, s := range *resp.JSON200 {
		out = append(out, Cluster{
			ID: s.Id.String(), Name: string(s.Name), Type: string(s.Type), Size: string(s.Size),
			Version: string(s.Version), Location: string(s.Location), Status: string(s.Status),
			URL: s.Url, AutoUpgradeEnabled: boolDeref(s.AutoUpgradeEnabled),
			CreatedAt: fmtTime(s.CreatedAt), UpdatedAt: fmtTime(s.UpdatedAt),
		})
	}
	return out, nil
}

// GetCluster returns a single cluster by ID.
func (c *Client) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	resp, err := c.gen.GetClusterWithResponse(ctx, cid(id), &apiclient.GetClusterParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return clusterFromAPI(resp.JSON200), nil
}

// CreateCluster creates a cluster.
func (c *Client) CreateCluster(ctx context.Context, req CreateClusterRequest) (*Cluster, error) {
	body := apiclient.CreateClusterJSONRequestBody{
		Name:               apiclient.ClusterName(req.Name),
		Size:               apiclient.ClusterSize(req.Size),
		Version:            apiclient.KeycloakVersion(req.Version),
		Location:           apiclient.ClusterLocation(req.Location),
		AutoUpgradeEnabled: req.AutoUpgradeEnabled,
	}
	if req.Type != "" {
		t := apiclient.ClusterType(req.Type)
		body.Type = &t
	}
	resp, err := c.gen.CreateClusterWithResponse(ctx, &apiclient.CreateClusterParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return clusterFromAPI(resp.JSON201), nil
}

// UpdateCluster patches mutable fields of a cluster.
func (c *Client) UpdateCluster(ctx context.Context, id string, req UpdateClusterRequest) (*Cluster, error) {
	body := apiclient.UpdateClusterJSONRequestBody{AutoUpgradeEnabled: req.AutoUpgradeEnabled}
	if req.Size != nil {
		s := apiclient.ClusterSize(*req.Size)
		body.Size = &s
	}
	if req.Version != nil {
		v := apiclient.KeycloakVersion(*req.Version)
		body.Version = &v
	}
	resp, err := c.gen.UpdateClusterWithResponse(ctx, cid(id), &apiclient.UpdateClusterParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return clusterFromAPI(resp.JSON200), nil
}

// DeleteCluster deletes a cluster.
func (c *Client) DeleteCluster(ctx context.Context, id string) error {
	resp, err := c.gen.DeleteClusterWithResponse(ctx, cid(id), &apiclient.DeleteClusterParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// WaitForClusterReady polls the cluster until it reaches "available", returns a
// terminal-failure error if it reaches "failed", or surfaces the context error.
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

// ---- Realms ----

// Realm mirrors the public API Realm resource.
type Realm struct {
	Name                        string
	DisplayName                 string
	Enabled                     bool
	SSLRequired                 string
	RegistrationAllowed         bool
	RegistrationEmailAsUsername bool
	LoginWithEmailAllowed       bool
	DuplicateEmailsAllowed      bool
	CreatedAt                   string
}

func realmFromAPI(r *apiclient.Realm) *Realm {
	out := &Realm{
		Name: string(r.Name), DisplayName: string(r.DisplayName), Enabled: r.Enabled,
		SSLRequired: string(r.SslRequired), RegistrationAllowed: r.RegistrationAllowed,
		RegistrationEmailAsUsername: r.RegistrationEmailAsUsername,
		LoginWithEmailAllowed:       r.LoginWithEmailAllowed,
		DuplicateEmailsAllowed:      r.DuplicateEmailsAllowed,
	}
	if r.CreatedAt != nil {
		out.CreatedAt = fmtTime(*r.CreatedAt)
	}
	return out
}

// ListRealms returns the realms in a cluster.
func (c *Client) ListRealms(ctx context.Context, clusterID string) ([]Realm, error) {
	resp, err := c.gen.ListRealmsWithResponse(ctx, cid(clusterID), &apiclient.ListRealmsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]Realm, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, *realmFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetRealm returns a single realm by name.
func (c *Client) GetRealm(ctx context.Context, clusterID, name string) (*Realm, error) {
	resp, err := c.gen.GetRealmWithResponse(ctx, cid(clusterID), apiclient.RealmName(name), &apiclient.GetRealmParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return realmFromAPI(resp.JSON200), nil
}

// CreateRealm creates a realm. The API accepts only name + display_name on
// create, so realm settings are applied with a follow-up update.
func (c *Client) CreateRealm(ctx context.Context, clusterID string, r Realm) (*Realm, error) {
	body := apiclient.CreateRealmJSONRequestBody{Name: apiclient.RealmName(r.Name)}
	if r.DisplayName != "" {
		dn := apiclient.RealmDisplayName(r.DisplayName)
		body.DisplayName = &dn
	}
	resp, err := c.gen.CreateRealmWithResponse(ctx, cid(clusterID), &apiclient.CreateRealmParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	// Apply the remaining settings (enabled, ssl_required, registration flags).
	return c.UpdateRealm(ctx, clusterID, r.Name, r)
}

// UpdateRealm updates a realm's mutable settings.
func (c *Client) UpdateRealm(ctx context.Context, clusterID, name string, r Realm) (*Realm, error) {
	enabled := r.Enabled
	reg := r.RegistrationAllowed
	regEmail := r.RegistrationEmailAsUsername
	loginEmail := r.LoginWithEmailAllowed
	dupEmails := r.DuplicateEmailsAllowed
	body := apiclient.UpdateRealmJSONRequestBody{
		Enabled:                     &enabled,
		RegistrationAllowed:         &reg,
		RegistrationEmailAsUsername: &regEmail,
		LoginWithEmailAllowed:       &loginEmail,
		DuplicateEmailsAllowed:      &dupEmails,
	}
	if r.DisplayName != "" {
		dn := apiclient.RealmDisplayName(r.DisplayName)
		body.DisplayName = &dn
	}
	if r.SSLRequired != "" {
		ssl := apiclient.RealmSslRequired(r.SSLRequired)
		body.SslRequired = &ssl
	}
	resp, err := c.gen.UpdateRealmWithResponse(ctx, cid(clusterID), apiclient.RealmName(name), &apiclient.UpdateRealmParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return realmFromAPI(resp.JSON200), nil
}

// DeleteRealm deletes a realm.
func (c *Client) DeleteRealm(ctx context.Context, clusterID, name string) error {
	resp, err := c.gen.DeleteRealmWithResponse(ctx, cid(clusterID), apiclient.RealmName(name), &apiclient.DeleteRealmParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- Applications ----

// Application mirrors the public API Application resource.
type Application struct {
	ClientID              string
	Name                  string
	Description           string
	Type                  string
	Protocol              string
	Status                string
	RedirectURIs          []string
	GrantTypes            []string
	PKCERequired          bool
	ConsentRequired       bool
	ServiceAccountEnabled bool
	ClientSecret          string
	CreatedAt             string
	UpdatedAt             string
}

func applicationFromAPI(a *apiclient.Application) *Application {
	out := &Application{
		ClientID: string(a.ClientId), Name: a.Name, Type: string(a.Type), Protocol: string(a.Protocol),
		Status: string(a.Status), RedirectURIs: a.RedirectUris, PKCERequired: a.PkceRequired,
		ConsentRequired: a.ConsentRequired, ServiceAccountEnabled: a.ServiceAccountEnabled,
		CreatedAt: fmtTime(a.CreatedAt), UpdatedAt: fmtTime(a.UpdatedAt),
	}
	if a.Description != nil {
		out.Description = *a.Description
	}
	if a.ClientSecret != nil {
		out.ClientSecret = *a.ClientSecret
	}
	for _, g := range a.GrantTypes {
		out.GrantTypes = append(out.GrantTypes, string(g))
	}
	return out
}

func grantTypes(in []string) []apiclient.GrantType {
	out := make([]apiclient.GrantType, 0, len(in))
	for _, g := range in {
		out = append(out, apiclient.GrantType(g))
	}
	return out
}

// ListApplications returns all applications in a realm, following pagination.
func (c *Client) ListApplications(ctx context.Context, clusterID, realm string) ([]Application, error) {
	const limit = 100
	var out []Application
	for offset := 0; ; offset += limit {
		l := apiclient.PaginationLimit(limit)
		o := apiclient.PaginationOffset(offset)
		resp, err := c.gen.ListApplicationsWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm),
			&apiclient.ListApplicationsParams{Limit: &l, Offset: &o, APIVersion: c.ver()})
		if err != nil {
			return nil, err
		}
		if resp.JSON200 == nil {
			return nil, statusError(resp.HTTPResponse, resp.Body)
		}
		page := *resp.JSON200
		for i := range page {
			out = append(out, *applicationFromAPI(&page[i]))
		}
		if len(page) < limit {
			break
		}
	}
	return out, nil
}

// RotateApplicationSecret rotates an application's client secret and returns the
// new value (only returned once).
func (c *Client) RotateApplicationSecret(ctx context.Context, clusterID, realm, clientID string) (string, error) {
	resp, err := c.gen.RotateApplicationSecretWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), &apiclient.RotateApplicationSecretParams{APIVersion: c.ver()})
	if err != nil {
		return "", err
	}
	if resp.JSON200 == nil {
		return "", statusError(resp.HTTPResponse, resp.Body)
	}
	return resp.JSON200.ClientSecret, nil
}

// GetApplication returns a single application by client ID.
func (c *Client) GetApplication(ctx context.Context, clusterID, realm, clientID string) (*Application, error) {
	resp, err := c.gen.GetApplicationWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), &apiclient.GetApplicationParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return applicationFromAPI(resp.JSON200), nil
}

// CreateApplication creates an application in a realm.
func (c *Client) CreateApplication(ctx context.Context, clusterID, realm string, a Application) (*Application, error) {
	pkce := a.PKCERequired
	consent := a.ConsentRequired
	body := apiclient.CreateApplicationJSONRequestBody{
		ClientId:        apiclient.ApplicationClientId(a.ClientID),
		Name:            a.Name,
		GrantTypes:      grantTypes(a.GrantTypes),
		PkceRequired:    &pkce,
		ConsentRequired: &consent,
		Description:     strPtr(a.Description),
	}
	if a.Type != "" {
		t := apiclient.ApplicationType(a.Type)
		body.Type = &t
	}
	if a.Protocol != "" {
		p := apiclient.ApplicationProtocol(a.Protocol)
		body.Protocol = &p
	}
	if len(a.RedirectURIs) > 0 {
		ru := a.RedirectURIs
		body.RedirectUris = &ru
	}
	resp, err := c.gen.CreateApplicationWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.CreateApplicationParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return applicationFromAPI(resp.JSON201), nil
}

// UpdateApplication updates an application's mutable fields.
func (c *Client) UpdateApplication(ctx context.Context, clusterID, realm, clientID string, a Application) (*Application, error) {
	pkce := a.PKCERequired
	consent := a.ConsentRequired
	name := a.Name
	body := apiclient.UpdateApplicationJSONRequestBody{
		Name:            &name,
		PkceRequired:    &pkce,
		ConsentRequired: &consent,
		Description:     strPtr(a.Description),
	}
	if len(a.GrantTypes) > 0 {
		gt := grantTypes(a.GrantTypes)
		body.GrantTypes = &gt
	}
	if len(a.RedirectURIs) > 0 {
		ru := a.RedirectURIs
		body.RedirectUris = &ru
	}
	resp, err := c.gen.UpdateApplicationWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), &apiclient.UpdateApplicationParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return applicationFromAPI(resp.JSON200), nil
}

// DeleteApplication deletes an application.
func (c *Client) DeleteApplication(ctx context.Context, clusterID, realm, clientID string) error {
	resp, err := c.gen.DeleteApplicationWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), &apiclient.DeleteApplicationParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- SMTP ----

// SMTPConfig mirrors the public API SmtpConfig (read) resource.
type SMTPConfig struct {
	Host            string
	Port            int64
	Encryption      string
	FromEmail       string
	FromName        string
	AuthType        string
	Username        string
	HasPassword     bool
	TokenURL        string
	TokenScope      string
	ClientID        string
	HasClientSecret bool
	Status          string
}

// UpsertSMTPRequest is the body for creating/updating SMTP config.
type UpsertSMTPRequest struct {
	Host         string
	Port         int64
	Encryption   string
	FromEmail    string
	FromName     string
	AuthType     string
	Username     string
	Password     string
	TokenURL     string
	TokenScope   string
	ClientID     string
	ClientSecret string
}

func smtpFromAPI(s *apiclient.SmtpConfig) *SMTPConfig {
	out := &SMTPConfig{
		Host: string(s.Host), Port: int64(s.Port), Encryption: string(s.Encryption),
		FromEmail: string(s.FromEmail), AuthType: string(s.AuthType),
		HasPassword: s.HasPassword, HasClientSecret: s.HasClientSecret, Status: string(s.Status),
	}
	if s.FromName != nil {
		out.FromName = *s.FromName
	}
	if s.Username != nil {
		out.Username = *s.Username
	}
	if s.TokenUrl != nil {
		out.TokenURL = *s.TokenUrl
	}
	if s.TokenScope != nil {
		out.TokenScope = *s.TokenScope
	}
	if s.ClientId != nil {
		out.ClientID = *s.ClientId
	}
	return out
}

// GetSMTP returns the SMTP configuration for a realm.
func (c *Client) GetSMTP(ctx context.Context, clusterID, realm string) (*SMTPConfig, error) {
	resp, err := c.gen.GetSmtpConfigWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.GetSmtpConfigParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return smtpFromAPI(resp.JSON200), nil
}

// UpsertSMTP creates or updates the SMTP configuration for a realm.
func (c *Client) UpsertSMTP(ctx context.Context, clusterID, realm string, req UpsertSMTPRequest) (*SMTPConfig, error) {
	body := apiclient.UpsertSmtpConfigJSONRequestBody{
		Host:      apiclient.SmtpHost(req.Host),
		Port:      apiclient.SmtpPort(req.Port),
		FromEmail: openapitypes.Email(req.FromEmail),
		AuthType:  apiclient.SmtpAuthType(req.AuthType),
	}
	if req.Encryption != "" {
		enc := apiclient.SmtpEncryption(req.Encryption)
		body.Encryption = &enc
	}
	body.FromName = strPtr(req.FromName)
	body.Username = strPtr(req.Username)
	body.Password = strPtr(req.Password)
	body.TokenUrl = strPtr(req.TokenURL)
	body.TokenScope = strPtr(req.TokenScope)
	body.ClientId = strPtr(req.ClientID)
	body.ClientSecret = strPtr(req.ClientSecret)

	resp, err := c.gen.UpsertSmtpConfigWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.UpsertSmtpConfigParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return smtpFromAPI(resp.JSON200), nil
}

// DeleteSMTP removes the SMTP configuration for a realm.
func (c *Client) DeleteSMTP(ctx context.Context, clusterID, realm string) error {
	resp, err := c.gen.DeleteSmtpConfigWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.DeleteSmtpConfigParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- Cluster metadata ----

// ClusterLocationInfo is a supported deployment region.
type ClusterLocationInfo struct {
	Location  string
	Name      string
	Available bool
}

// ClusterTypeInfo is a supported cluster type.
type ClusterTypeInfo struct {
	Type      string
	Name      string
	Available bool
}

// ClusterFeatureInfo is a Keycloak feature flag available to tenant clusters.
type ClusterFeatureInfo struct {
	Name        string
	DisplayName string
	Description *string
	Preview     bool
	MinVersion  *string
	MaxVersion  *string
}

// ListClusterLocations returns the supported deployment regions.
func (c *Client) ListClusterLocations(ctx context.Context) ([]ClusterLocationInfo, error) {
	resp, err := c.gen.ListClusterLocationsWithResponse(ctx, &apiclient.ListClusterLocationsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]ClusterLocationInfo, 0, len(*resp.JSON200))
	for _, l := range *resp.JSON200 {
		out = append(out, ClusterLocationInfo{Location: string(l.Location), Name: l.Name, Available: l.Available})
	}
	return out, nil
}

// ListClusterTypes returns the supported cluster types.
func (c *Client) ListClusterTypes(ctx context.Context) ([]ClusterTypeInfo, error) {
	resp, err := c.gen.ListClusterTypesWithResponse(ctx, &apiclient.ListClusterTypesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]ClusterTypeInfo, 0, len(*resp.JSON200))
	for _, t := range *resp.JSON200 {
		out = append(out, ClusterTypeInfo{Type: string(t.Type), Name: t.Name, Available: t.Available})
	}
	return out, nil
}

// ListClusterFeatures returns the available Keycloak feature flags.
func (c *Client) ListClusterFeatures(ctx context.Context) ([]ClusterFeatureInfo, error) {
	resp, err := c.gen.ListClusterFeaturesWithResponse(ctx, &apiclient.ListClusterFeaturesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]ClusterFeatureInfo, 0, len(*resp.JSON200))
	for _, f := range *resp.JSON200 {
		info := ClusterFeatureInfo{Name: f.Name, DisplayName: f.DisplayName, Preview: f.Preview}
		if !f.Description.IsNull() {
			if v, err := f.Description.Get(); err == nil {
				info.Description = &v
			}
		}
		if !f.MinVersion.IsNull() {
			if v, err := f.MinVersion.Get(); err == nil {
				s := string(v)
				info.MinVersion = &s
			}
		}
		if !f.MaxVersion.IsNull() {
			if v, err := f.MaxVersion.Get(); err == nil {
				s := string(v)
				info.MaxVersion = &s
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// ---- Identity providers ----

// IdentityProvider mirrors the public API identity-provider resource with its
// structured configuration.
type IdentityProvider struct {
	ProviderID        string
	Type              string
	DisplayName       string
	Enabled           bool
	ClientID          string
	ClientSecret      string // write-only; never returned on read
	ExternallyManaged bool
	Config            ProviderConfig
	CreatedAt         string
	UpdatedAt         string
}

// ProviderConfig is the structured identity-provider configuration.
type ProviderConfig struct {
	ButtonText        string
	IconURL           string
	SyncMode          string
	TrustEmail        *bool
	AttributeMappings map[string]string
	OIDC              *OIDCConfig
	LDAP              *LDAPConfig
	SAML              *SAMLConfig
}

// OIDCConfig holds OIDC endpoint configuration.
type OIDCConfig struct {
	AuthorizationURL string
	Issuer           string
	LogoutURL        string
	TokenURL         string
	UserinfoURL      string
}

// LDAPConfig holds LDAP directory configuration.
type LDAPConfig struct {
	BaseDN    string
	BindDN    string
	ServerURL string
}

// SAMLConfig holds SAML provider configuration.
type SAMLConfig struct {
	EntityID    string
	MetadataURL string
	MetadataXML string
	SSOURL      string
}

func deref(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

func toAPIProviderConfig(c ProviderConfig) *apiclient.ProviderConfig {
	out := &apiclient.ProviderConfig{
		ButtonText: strPtr(c.ButtonText),
		IconUrl:    strPtr(c.IconURL),
		TrustEmail: c.TrustEmail,
	}
	if c.SyncMode != "" {
		sm := apiclient.SyncMode(c.SyncMode)
		out.SyncMode = &sm
	}
	if len(c.AttributeMappings) > 0 {
		m := c.AttributeMappings
		out.AttributeMappings = &m
	}
	if c.OIDC != nil {
		out.Oidc = &apiclient.OIDCConfig{
			AuthorizationUrl: strPtr(c.OIDC.AuthorizationURL),
			Issuer:           strPtr(c.OIDC.Issuer),
			LogoutUrl:        strPtr(c.OIDC.LogoutURL),
			TokenUrl:         strPtr(c.OIDC.TokenURL),
			UserinfoUrl:      strPtr(c.OIDC.UserinfoURL),
		}
	}
	if c.LDAP != nil {
		out.Ldap = &apiclient.LDAPConfig{
			BaseDn:    strPtr(c.LDAP.BaseDN),
			BindDn:    strPtr(c.LDAP.BindDN),
			ServerUrl: strPtr(c.LDAP.ServerURL),
		}
	}
	if c.SAML != nil {
		out.Saml = &apiclient.SAMLIdPConfig{
			EntityId:    strPtr(c.SAML.EntityID),
			MetadataUrl: strPtr(c.SAML.MetadataURL),
			MetadataXml: strPtr(c.SAML.MetadataXML),
			SsoUrl:      strPtr(c.SAML.SSOURL),
		}
	}
	return out
}

func providerConfigFromAPI(c apiclient.ProviderConfig) ProviderConfig {
	out := ProviderConfig{
		ButtonText: deref(c.ButtonText),
		IconURL:    deref(c.IconUrl),
		TrustEmail: c.TrustEmail,
	}
	if c.SyncMode != nil {
		out.SyncMode = string(*c.SyncMode)
	}
	if c.AttributeMappings != nil {
		out.AttributeMappings = *c.AttributeMappings
	}
	if c.Oidc != nil {
		out.OIDC = &OIDCConfig{
			AuthorizationURL: deref(c.Oidc.AuthorizationUrl), Issuer: deref(c.Oidc.Issuer),
			LogoutURL: deref(c.Oidc.LogoutUrl), TokenURL: deref(c.Oidc.TokenUrl), UserinfoURL: deref(c.Oidc.UserinfoUrl),
		}
	}
	if c.Ldap != nil {
		out.LDAP = &LDAPConfig{BaseDN: deref(c.Ldap.BaseDn), BindDN: deref(c.Ldap.BindDn), ServerURL: deref(c.Ldap.ServerUrl)}
	}
	if c.Saml != nil {
		out.SAML = &SAMLConfig{EntityID: deref(c.Saml.EntityId), MetadataURL: deref(c.Saml.MetadataUrl), MetadataXML: deref(c.Saml.MetadataXml), SSOURL: deref(c.Saml.SsoUrl)}
	}
	return out
}

func idpFromAPI(p *apiclient.IdentityProvider) *IdentityProvider {
	return &IdentityProvider{
		ProviderID: string(p.ProviderId), Type: string(p.Type), DisplayName: p.DisplayName,
		Enabled: p.Enabled, ClientID: deref(p.ClientId), ExternallyManaged: p.ExternallyManaged,
		Config: providerConfigFromAPI(p.Config), CreatedAt: fmtTime(p.CreatedAt), UpdatedAt: fmtTime(p.UpdatedAt),
	}
}

// ListIdentityProviders returns the identity providers in a realm.
func (c *Client) ListIdentityProviders(ctx context.Context, clusterID, realm string) ([]IdentityProvider, error) {
	resp, err := c.gen.ListIdentityProvidersWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.ListIdentityProvidersParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]IdentityProvider, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, *idpFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetIdentityProvider returns a single identity provider by its provider ID.
func (c *Client) GetIdentityProvider(ctx context.Context, clusterID, realm, providerID string) (*IdentityProvider, error) {
	resp, err := c.gen.GetIdentityProviderWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ProviderId(providerID), &apiclient.GetIdentityProviderParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return idpFromAPI(resp.JSON200), nil
}

// CreateIdentityProvider creates an identity provider. Create does not accept
// `enabled`, so it is applied with a follow-up update.
func (c *Client) CreateIdentityProvider(ctx context.Context, clusterID, realm string, idp IdentityProvider) (*IdentityProvider, error) {
	body := apiclient.CreateIdentityProviderJSONRequestBody{
		ProviderId:   apiclient.SkycloakProviderId(idp.ProviderID),
		Type:         apiclient.ProviderType(idp.Type),
		DisplayName:  idp.DisplayName,
		ClientId:     strPtr(idp.ClientID),
		ClientSecret: strPtr(idp.ClientSecret),
		Config:       toAPIProviderConfig(idp.Config),
	}
	resp, err := c.gen.CreateIdentityProviderWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.CreateIdentityProviderParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return c.UpdateIdentityProvider(ctx, clusterID, realm, idp.ProviderID, idp)
}

// UpdateIdentityProvider updates an identity provider's mutable fields.
func (c *Client) UpdateIdentityProvider(ctx context.Context, clusterID, realm, providerID string, idp IdentityProvider) (*IdentityProvider, error) {
	enabled := idp.Enabled
	displayName := idp.DisplayName
	body := apiclient.UpdateIdentityProviderJSONRequestBody{
		Enabled:     &enabled,
		DisplayName: &displayName,
		Config:      toAPIProviderConfig(idp.Config),
	}
	if idp.ClientID != "" {
		body.ClientId = &idp.ClientID
	}
	if idp.ClientSecret != "" {
		body.ClientSecret = &idp.ClientSecret
	}
	resp, err := c.gen.UpdateIdentityProviderWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ProviderId(providerID), &apiclient.UpdateIdentityProviderParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return idpFromAPI(resp.JSON200), nil
}

// DeleteIdentityProvider deletes an identity provider.
func (c *Client) DeleteIdentityProvider(ctx context.Context, clusterID, realm, providerID string) error {
	resp, err := c.gen.DeleteIdentityProviderWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ProviderId(providerID), &apiclient.DeleteIdentityProviderParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- Custom domains ----

func uid(s string) uuid.UUID { id, _ := uuid.Parse(s); return id }

// DNSRecord is a DNS record the customer must create to verify/route a domain.
type DNSRecord struct {
	Type  string
	Name  string
	Value string
}

// Domain mirrors the public API custom-domain resource.
type Domain struct {
	ID                 string
	ClusterID          string
	Domain             string
	Subdomain          string
	CnameTarget        string
	SSLStatus          string
	VerificationStatus string
	IsActive           bool
	DNSRecords         []DNSRecord
	CreatedAt          string
	UpdatedAt          string
}

// CreateDomainRequest is the body for adding a custom domain.
type CreateDomainRequest struct {
	Domain    string
	Subdomain string
}

func domainFromAPI(d *apiclient.Domain) *Domain {
	out := &Domain{
		ID: d.Id.String(), ClusterID: d.ClusterId.String(), Domain: string(d.Domain),
		CnameTarget: d.CnameTarget, SSLStatus: string(d.SslStatus), VerificationStatus: string(d.VerificationStatus),
		IsActive: d.IsActive, CreatedAt: fmtTime(d.CreatedAt), UpdatedAt: fmtTime(d.UpdatedAt),
	}
	if d.Subdomain != nil {
		out.Subdomain = *d.Subdomain
	}
	for _, r := range d.DnsRecords {
		out.DNSRecords = append(out.DNSRecords, DNSRecord{Type: string(r.Type), Name: r.Name, Value: r.Value})
	}
	return out
}

// ListDomains returns the custom domains on a cluster.
func (c *Client) ListDomains(ctx context.Context, clusterID string) ([]Domain, error) {
	resp, err := c.gen.ListDomainsWithResponse(ctx, cid(clusterID), &apiclient.ListDomainsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]Domain, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, *domainFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetDomain returns a single custom domain by ID.
func (c *Client) GetDomain(ctx context.Context, clusterID, domainID string) (*Domain, error) {
	resp, err := c.gen.GetDomainWithResponse(ctx, cid(clusterID), uid(domainID), &apiclient.GetDomainParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return domainFromAPI(resp.JSON200), nil
}

// CreateDomain adds a custom domain to a cluster.
func (c *Client) CreateDomain(ctx context.Context, clusterID string, req CreateDomainRequest) (*Domain, error) {
	body := apiclient.CreateDomainJSONRequestBody{Domain: req.Domain}
	if req.Subdomain != "" {
		body.Subdomain = &req.Subdomain
	}
	resp, err := c.gen.CreateDomainWithResponse(ctx, cid(clusterID), &apiclient.CreateDomainParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return domainFromAPI(resp.JSON201), nil
}

// DeleteDomain removes a custom domain.
func (c *Client) DeleteDomain(ctx context.Context, clusterID, domainID string) error {
	resp, err := c.gen.DeleteDomainWithResponse(ctx, cid(clusterID), uid(domainID), &apiclient.DeleteDomainParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// VerifyDomain triggers DNS verification for a domain and returns its updated state.
func (c *Client) VerifyDomain(ctx context.Context, clusterID, domainID string) (*Domain, error) {
	resp, err := c.gen.VerifyDomainWithResponse(ctx, cid(clusterID), uid(domainID), &apiclient.VerifyDomainParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return domainFromAPI(resp.JSON200), nil
}

// DomainRoute maps a realm onto a custom domain.
type DomainRoute struct {
	ID                 string
	ClusterID          string
	DomainID           string
	Realm              string
	AllowAdminAccess   bool
	HideRealmPath      bool
	CorsAllowedOrigins []string
	CreatedAt          string
	UpdatedAt          string
}

// DomainRouteInput holds the mutable fields for creating/updating a route.
type DomainRouteInput struct {
	Realm              string
	AllowAdminAccess   bool
	HideRealmPath      bool
	CorsAllowedOrigins []string
}

func domainRouteFromAPI(r *apiclient.DomainRoute) *DomainRoute {
	return &DomainRoute{
		ID: r.Id.String(), ClusterID: r.ClusterId.String(), DomainID: r.DomainId.String(), Realm: string(r.Realm),
		AllowAdminAccess: r.AllowAdminAccess, HideRealmPath: r.HideRealmPath, CorsAllowedOrigins: r.CorsAllowedOrigins,
		CreatedAt: fmtTime(r.CreatedAt), UpdatedAt: fmtTime(r.UpdatedAt),
	}
}

// GetDomainRoute returns a single route.
func (c *Client) GetDomainRoute(ctx context.Context, clusterID, domainID, routeID string) (*DomainRoute, error) {
	resp, err := c.gen.GetDomainRouteWithResponse(ctx, cid(clusterID), uid(domainID), uid(routeID), &apiclient.GetDomainRouteParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return domainRouteFromAPI(resp.JSON200), nil
}

// CreateDomainRoute adds a realm route to a domain.
func (c *Client) CreateDomainRoute(ctx context.Context, clusterID, domainID string, in DomainRouteInput) (*DomainRoute, error) {
	admin := in.AllowAdminAccess
	hide := in.HideRealmPath
	body := apiclient.CreateDomainRouteJSONRequestBody{
		Realm: apiclient.RealmName(in.Realm), AllowAdminAccess: &admin, HideRealmPath: &hide,
	}
	if len(in.CorsAllowedOrigins) > 0 {
		o := in.CorsAllowedOrigins
		body.CorsAllowedOrigins = &o
	}
	resp, err := c.gen.CreateDomainRouteWithResponse(ctx, cid(clusterID), uid(domainID), &apiclient.CreateDomainRouteParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return domainRouteFromAPI(resp.JSON201), nil
}

// UpdateDomainRoute updates a route's mutable fields.
func (c *Client) UpdateDomainRoute(ctx context.Context, clusterID, domainID, routeID string, in DomainRouteInput) (*DomainRoute, error) {
	admin := in.AllowAdminAccess
	o := in.CorsAllowedOrigins
	// hide_realm_path is create-only on the API; it is not updatable.
	body := apiclient.UpdateDomainRouteJSONRequestBody{AllowAdminAccess: &admin, CorsAllowedOrigins: &o}
	resp, err := c.gen.UpdateDomainRouteWithResponse(ctx, cid(clusterID), uid(domainID), uid(routeID), &apiclient.UpdateDomainRouteParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return domainRouteFromAPI(resp.JSON200), nil
}

// DeleteDomainRoute removes a route.
func (c *Client) DeleteDomainRoute(ctx context.Context, clusterID, domainID, routeID string) error {
	resp, err := c.gen.DeleteDomainRouteWithResponse(ctx, cid(clusterID), uid(domainID), uid(routeID), &apiclient.DeleteDomainRouteParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- Branding & themes ----

// LoginI18n configures login-page internationalization.
type LoginI18n struct {
	Enabled                  bool
	DefaultLocale            string
	SupportedLocales         []string
	LanguageSelectionMode    string
	LanguageSelectorPosition string
	LanguageSelectorStyle    string
}

// SSOConfig configures identity-provider button display on the login page.
type SSOConfig struct {
	Enabled      bool
	ButtonSize   string
	DisplayStyle string
	Layout       string
}

// LoginBranding mirrors the public API login-branding resource.
type LoginBranding struct {
	ClusterID, Realm                    string
	PrimaryColor, BackgroundColor       string
	LogoURL, FaviconURL, FontURL        string
	PrivacyPolicyURL, TermsOfServiceURL string
	ForgotPasswordEnabled               bool
	RegistrationEnabled                 bool
	RememberMeEnabled                   bool
	ShowPoweredBy                       bool
	Internationalization                *LoginI18n
	SSO                                 *SSOConfig
	Status                              string
	AppliedAt, CreatedAt, UpdatedAt     string
}

// UpsertLoginBrandingRequest is the body for creating/updating login branding.
// The toggle pointers are nil when the caller leaves them unset, so the API
// applies its own default rather than being forced to a value.
type UpsertLoginBrandingRequest struct {
	PrimaryColor, BackgroundColor       string
	LogoURL, FaviconURL, FontURL        string
	PrivacyPolicyURL, TermsOfServiceURL string
	ForgotPasswordEnabled               *bool
	RegistrationEnabled                 *bool
	RememberMeEnabled                   *bool
	ShowPoweredBy                       *bool
	Internationalization                *LoginI18n
	SSO                                 *SSOConfig
}

func loginBrandingFromAPI(b *apiclient.LoginBranding) *LoginBranding {
	out := &LoginBranding{
		ClusterID: b.ClusterId.String(), Realm: string(b.Realm),
		ForgotPasswordEnabled: b.ForgotPasswordEnabled, RegistrationEnabled: b.RegistrationEnabled,
		RememberMeEnabled: b.RememberMeEnabled, ShowPoweredBy: b.ShowPoweredBy,
		Status: string(b.Status), CreatedAt: fmtTime(b.CreatedAt), UpdatedAt: fmtTime(b.UpdatedAt),
	}
	if b.AppliedAt != nil {
		out.AppliedAt = fmtTime(*b.AppliedAt)
	}
	out.PrimaryColor = derefStr(b.PrimaryColor)
	out.BackgroundColor = derefStr(b.BackgroundColor)
	out.LogoURL = derefStr(b.LogoUrl)
	out.FaviconURL = derefStr(b.FaviconUrl)
	out.FontURL = derefStr(b.FontUrl)
	out.PrivacyPolicyURL = derefStr(b.PrivacyPolicyUrl)
	out.TermsOfServiceURL = derefStr(b.TermsOfServiceUrl)
	if i := b.Internationalization; i != nil {
		out.Internationalization = &LoginI18n{
			Enabled: i.Enabled, DefaultLocale: i.DefaultLocale, SupportedLocales: i.SupportedLocales,
			LanguageSelectionMode:    string(i.LanguageSelectionMode),
			LanguageSelectorPosition: string(i.LanguageSelectorPosition),
			LanguageSelectorStyle:    string(i.LanguageSelectorStyle),
		}
	}
	if s := b.Sso; s != nil {
		out.SSO = &SSOConfig{Enabled: s.Enabled, ButtonSize: string(s.ButtonSize), DisplayStyle: string(s.DisplayStyle), Layout: string(s.Layout)}
	}
	return out
}

// GetLoginBranding returns the login-branding configuration for a realm.
func (c *Client) GetLoginBranding(ctx context.Context, clusterID, realm string) (*LoginBranding, error) {
	resp, err := c.gen.GetLoginBrandingWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.GetLoginBrandingParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return loginBrandingFromAPI(resp.JSON200), nil
}

// UpsertLoginBranding creates or updates the login-branding configuration.
func (c *Client) UpsertLoginBranding(ctx context.Context, clusterID, realm string, req UpsertLoginBrandingRequest) (*LoginBranding, error) {
	body := apiclient.UpsertLoginBrandingJSONRequestBody{
		ForgotPasswordEnabled: req.ForgotPasswordEnabled,
		RegistrationEnabled:   req.RegistrationEnabled,
		RememberMeEnabled:     req.RememberMeEnabled,
		ShowPoweredBy:         req.ShowPoweredBy,
		PrimaryColor:          strPtr(req.PrimaryColor),
		BackgroundColor:       strPtr(req.BackgroundColor),
		LogoUrl:               strPtr(req.LogoURL),
		FaviconUrl:            strPtr(req.FaviconURL),
		FontUrl:               strPtr(req.FontURL),
		PrivacyPolicyUrl:      strPtr(req.PrivacyPolicyURL),
		TermsOfServiceUrl:     strPtr(req.TermsOfServiceURL),
	}
	if i := req.Internationalization; i != nil {
		body.Internationalization = &apiclient.LoginInternationalizationConfig{
			Enabled: i.Enabled, DefaultLocale: i.DefaultLocale, SupportedLocales: i.SupportedLocales,
			LanguageSelectionMode:    apiclient.LanguageSelectionMode(i.LanguageSelectionMode),
			LanguageSelectorPosition: apiclient.LanguageSelectorPosition(i.LanguageSelectorPosition),
			LanguageSelectorStyle:    apiclient.LanguageSelectorStyle(i.LanguageSelectorStyle),
		}
	}
	if s := req.SSO; s != nil {
		body.Sso = &apiclient.SsoConfig{
			Enabled: s.Enabled, ButtonSize: apiclient.SsoButtonSize(s.ButtonSize),
			DisplayStyle: apiclient.SsoDisplayStyle(s.DisplayStyle), Layout: apiclient.SsoLayout(s.Layout),
		}
	}
	resp, err := c.gen.UpsertLoginBrandingWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.UpsertLoginBrandingParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return loginBrandingFromAPI(resp.JSON200), nil
}

// DeleteLoginBranding removes the login-branding configuration (reverts to defaults).
func (c *Client) DeleteLoginBranding(ctx context.Context, clusterID, realm string) error {
	resp, err := c.gen.DeleteLoginBrandingWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.DeleteLoginBrandingParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// EmailI18n configures email-template internationalization.
type EmailI18n struct {
	Enabled          bool
	DefaultLocale    string
	SupportedLocales []string
}

// EmailBranding mirrors the public API email-branding resource.
type EmailBranding struct {
	ClusterID, Realm                          string
	PrimaryColor                              string
	HeaderLogoLightURL, HeaderLogoDarkURL     string
	FooterText, FooterCompanyName, CompanyURL string
	Internationalization                      *EmailI18n
	Status                                    string
	AppliedAt, CreatedAt, UpdatedAt           string
}

// UpsertEmailBrandingRequest is the body for creating/updating email branding.
type UpsertEmailBrandingRequest struct {
	PrimaryColor                              string
	HeaderLogoLightURL, HeaderLogoDarkURL     string
	FooterText, FooterCompanyName, CompanyURL string
	Internationalization                      *EmailI18n
}

func emailBrandingFromAPI(b *apiclient.EmailBranding) *EmailBranding {
	out := &EmailBranding{
		ClusterID: b.ClusterId.String(), Realm: string(b.Realm),
		Status: string(b.Status), CreatedAt: fmtTime(b.CreatedAt), UpdatedAt: fmtTime(b.UpdatedAt),
	}
	if b.AppliedAt != nil {
		out.AppliedAt = fmtTime(*b.AppliedAt)
	}
	out.PrimaryColor = derefStr(b.PrimaryColor)
	out.HeaderLogoLightURL = derefStr(b.HeaderLogoLightUrl)
	out.HeaderLogoDarkURL = derefStr(b.HeaderLogoDarkUrl)
	out.FooterText = derefStr(b.FooterText)
	out.FooterCompanyName = derefStr(b.FooterCompanyName)
	out.CompanyURL = derefStr(b.CompanyUrl)
	if i := b.Internationalization; i != nil {
		out.Internationalization = &EmailI18n{Enabled: i.Enabled, DefaultLocale: i.DefaultLocale, SupportedLocales: i.SupportedLocales}
	}
	return out
}

// GetEmailBranding returns the email-branding configuration for a realm.
func (c *Client) GetEmailBranding(ctx context.Context, clusterID, realm string) (*EmailBranding, error) {
	resp, err := c.gen.GetEmailBrandingWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.GetEmailBrandingParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return emailBrandingFromAPI(resp.JSON200), nil
}

// UpsertEmailBranding creates or updates the email-branding configuration.
func (c *Client) UpsertEmailBranding(ctx context.Context, clusterID, realm string, req UpsertEmailBrandingRequest) (*EmailBranding, error) {
	body := apiclient.UpsertEmailBrandingJSONRequestBody{
		PrimaryColor:       strPtr(req.PrimaryColor),
		HeaderLogoLightUrl: strPtr(req.HeaderLogoLightURL),
		HeaderLogoDarkUrl:  strPtr(req.HeaderLogoDarkURL),
		FooterText:         strPtr(req.FooterText),
		FooterCompanyName:  strPtr(req.FooterCompanyName),
		CompanyUrl:         strPtr(req.CompanyURL),
	}
	if i := req.Internationalization; i != nil {
		body.Internationalization = &apiclient.InternationalizationConfig{
			Enabled: i.Enabled, DefaultLocale: i.DefaultLocale, SupportedLocales: i.SupportedLocales,
		}
	}
	resp, err := c.gen.UpsertEmailBrandingWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.UpsertEmailBrandingParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return emailBrandingFromAPI(resp.JSON200), nil
}

// DeleteEmailBranding removes the email-branding configuration (reverts to defaults).
func (c *Client) DeleteEmailBranding(ctx context.Context, clusterID, realm string) error {
	resp, err := c.gen.DeleteEmailBrandingWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.DeleteEmailBrandingParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// Theme mirrors the public API custom-theme resource.
type Theme struct {
	ID, ClusterID, Name, Description, Version, Status string
	ThemeTypes                                        []string
	FileSize                                          int64
	ErrorMessage                                      string
	DeployedAt, CreatedAt, UpdatedAt                  string
}

func themeFromAPI(t *apiclient.Theme) Theme {
	out := Theme{
		ID: t.Id.String(), ClusterID: t.ClusterId.String(), Name: t.Name, Status: string(t.Status),
		FileSize: t.FileSize, CreatedAt: fmtTime(t.CreatedAt), UpdatedAt: fmtTime(t.UpdatedAt),
	}
	out.Description = derefStr(t.Description)
	out.Version = derefStr(t.Version)
	out.ErrorMessage = derefStr(t.ErrorMessage)
	if t.DeployedAt != nil {
		out.DeployedAt = fmtTime(*t.DeployedAt)
	}
	for _, tt := range t.ThemeTypes {
		out.ThemeTypes = append(out.ThemeTypes, string(tt))
	}
	return out
}

// ListThemes returns the custom themes uploaded to a cluster.
func (c *Client) ListThemes(ctx context.Context, clusterID string) ([]Theme, error) {
	resp, err := c.gen.ListThemesWithResponse(ctx, cid(clusterID), &apiclient.ListThemesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]Theme, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, themeFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetTheme returns a single custom theme by ID.
func (c *Client) GetTheme(ctx context.Context, clusterID, themeID string) (*Theme, error) {
	resp, err := c.gen.GetThemeWithResponse(ctx, cid(clusterID), uid(themeID), &apiclient.GetThemeParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	t := themeFromAPI(resp.JSON200)
	return &t, nil
}

// WaitForThemeDeployed polls a just-uploaded theme until it reaches
// "deployed". Upload returns while Keycloak is still extracting and rolling
// out the package (status "deploying"); only a deployed theme can be
// assigned to a realm or application client.
func (c *Client) WaitForThemeDeployed(ctx context.Context, clusterID, themeID string) (*Theme, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		t, err := c.GetTheme(ctx, clusterID, themeID)
		if err != nil {
			return nil, err
		}
		switch t.Status {
		case "deployed":
			return t, nil
		case "failed":
			return t, fmt.Errorf("theme %s failed to deploy: %s", themeID, t.ErrorMessage)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// ThemeAssignment is the active theme per Keycloak theme type for a realm. An
// empty field means the realm uses Keycloak's built-in default.
type ThemeAssignment struct {
	Login, Account, Admin, Email string
}

func themeAssignmentFromAPI(a *apiclient.ThemeAssignment) *ThemeAssignment {
	return &ThemeAssignment{
		Login:   nThemeID(a.Login),
		Account: nThemeID(a.Account),
		Admin:   nThemeID(a.Admin),
		Email:   nThemeID(a.Email),
	}
}

// GetThemeAssignment returns the realm-level theme assignment.
func (c *Client) GetThemeAssignment(ctx context.Context, clusterID, realm string) (*ThemeAssignment, error) {
	resp, err := c.gen.GetThemeAssignmentWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.GetThemeAssignmentParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return themeAssignmentFromAPI(resp.JSON200), nil
}

// SetThemeAssignment sets the realm-level theme assignment. Empty fields are
// sent as explicit null (reset to Keycloak's built-in default).
func (c *Client) SetThemeAssignment(ctx context.Context, clusterID, realm string, a ThemeAssignment) (*ThemeAssignment, error) {
	body := apiclient.SetThemeAssignmentJSONRequestBody{
		Login:   themeIDNullable(a.Login),
		Account: themeIDNullable(a.Account),
		Admin:   themeIDNullable(a.Admin),
		Email:   themeIDNullable(a.Email),
	}
	resp, err := c.gen.SetThemeAssignmentWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.SetThemeAssignmentParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return themeAssignmentFromAPI(resp.JSON200), nil
}

// ClientThemeAssignment is the login theme override for a single client. An
// empty field means the client uses the realm default.
type ClientThemeAssignment struct {
	Login string
}

// GetClientThemeAssignment returns a client's login-theme override.
func (c *Client) GetClientThemeAssignment(ctx context.Context, clusterID, realm, clientID string) (*ClientThemeAssignment, error) {
	resp, err := c.gen.GetClientThemeAssignmentWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), &apiclient.GetClientThemeAssignmentParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return &ClientThemeAssignment{Login: nThemeID(resp.JSON200.Login)}, nil
}

// SetClientThemeAssignment sets a client's login-theme override. An empty login
// is sent as explicit null (reset to the realm default).
func (c *Client) SetClientThemeAssignment(ctx context.Context, clusterID, realm, clientID, login string) (*ClientThemeAssignment, error) {
	body := apiclient.SetClientThemeAssignmentJSONRequestBody{Login: themeIDNullable(login)}
	resp, err := c.gen.SetClientThemeAssignmentWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), &apiclient.SetClientThemeAssignmentParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return &ClientThemeAssignment{Login: nThemeID(resp.JSON200.Login)}, nil
}

// derefStr returns the pointed-to string, or "" if nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nThemeID extracts a theme ID string from a nullable, returning "" for
// null/unspecified.
func nThemeID(n nullable.Nullable[apiclient.ThemeId]) string {
	if !n.IsSpecified() || n.IsNull() {
		return ""
	}
	v, err := n.Get()
	if err != nil {
		return ""
	}
	return v.String()
}

// themeIDNullable maps "" to explicit null, otherwise to the parsed theme ID.
func themeIDNullable(s string) nullable.Nullable[apiclient.ThemeId] {
	if s == "" {
		return nullable.NewNullNullable[apiclient.ThemeId]()
	}
	return nullable.NewNullableWithValue(uid(s))
}

// ---- Extensions ----

// nStr extracts a string from a nullable, returning "" for null/unspecified.
func nStr(n nullable.Nullable[string]) string {
	if !n.IsSpecified() || n.IsNull() {
		return ""
	}
	v, err := n.Get()
	if err != nil {
		return ""
	}
	return v
}

// ExtensionInfo is a catalog entry from the extensions marketplace.
type ExtensionInfo struct {
	ID               string
	Name             string
	Description      string
	Source           string
	KeycloakVersions []string
	DocumentationURL string
	RepositoryURL    string
	IconURL          string
	ParameterType    string
	ScanStatus       string
	CreatedAt        string
	UpdatedAt        string
}

func nScanStatus(n nullable.Nullable[apiclient.ExtensionScanStatus]) string {
	if !n.IsSpecified() || n.IsNull() {
		return ""
	}
	v, err := n.Get()
	if err != nil {
		return ""
	}
	return string(v)
}

func extensionInfoFromAPI(e *apiclient.Extension) ExtensionInfo {
	return ExtensionInfo{
		ID: e.Id.String(), Name: e.Name, Description: nStr(e.Description), Source: string(e.Source),
		KeycloakVersions: e.KeycloakVersions, DocumentationURL: nStr(e.DocumentationUrl),
		RepositoryURL: nStr(e.RepositoryUrl), IconURL: nStr(e.IconUrl), ParameterType: string(e.ParameterType),
		ScanStatus: nScanStatus(e.ScanStatus), CreatedAt: fmtTime(e.CreatedAt), UpdatedAt: fmtTime(e.UpdatedAt),
	}
}

// ListExtensions returns the extension catalog available to the workspace.
func (c *Client) ListExtensions(ctx context.Context) ([]ExtensionInfo, error) {
	resp, err := c.gen.ListExtensionsWithResponse(ctx, &apiclient.ListExtensionsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]ExtensionInfo, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, extensionInfoFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetExtension returns a single catalog extension by ID.
func (c *Client) GetExtension(ctx context.Context, extensionID string) (*ExtensionInfo, error) {
	resp, err := c.gen.GetExtensionWithResponse(ctx, uid(extensionID), &apiclient.GetExtensionParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	e := extensionInfoFromAPI(resp.JSON200)
	return &e, nil
}

// ClusterExtension is an extension installed on a cluster.
type ClusterExtension struct {
	ExtensionID      string
	ExtensionName    string
	Source           string
	InstalledVersion string
	AvailableVersion string
	Status           string
	UpgradeAvailable bool
	InstalledAt      string
}

func clusterExtensionFromAPI(e *apiclient.ClusterExtension) *ClusterExtension {
	return &ClusterExtension{
		ExtensionID: e.ExtensionId.String(), ExtensionName: e.ExtensionName, Source: string(e.ExtensionSource),
		InstalledVersion: e.InstalledVersion, AvailableVersion: nStr(e.AvailableVersion), Status: string(e.Status),
		UpgradeAvailable: e.UpgradeAvailable, InstalledAt: fmtTime(e.InstalledAt),
	}
}

// ListClusterExtensions returns the extensions installed on a cluster.
func (c *Client) ListClusterExtensions(ctx context.Context, clusterID string) ([]ClusterExtension, error) {
	resp, err := c.gen.ListClusterExtensionsWithResponse(ctx, cid(clusterID), &apiclient.ListClusterExtensionsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]ClusterExtension, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, *clusterExtensionFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetClusterExtension returns a single installed extension by ID, or a 404
// APIError if it is not installed.
func (c *Client) GetClusterExtension(ctx context.Context, clusterID, extensionID string) (*ClusterExtension, error) {
	exts, err := c.ListClusterExtensions(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	for i := range exts {
		if exts[i].ExtensionID == extensionID {
			return &exts[i], nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound}
}

// InstallExtension installs an extension on a cluster. The operation is
// asynchronous; the returned extension may still be provisioning.
func (c *Client) InstallExtension(ctx context.Context, clusterID, extensionID string, params map[string]string) (*ClusterExtension, error) {
	body := apiclient.InstallExtensionJSONRequestBody{ExtensionId: uid(extensionID)}
	if len(params) > 0 {
		body.Parameters = &params
	}
	resp, err := c.gen.InstallExtensionWithResponse(ctx, cid(clusterID), &apiclient.InstallExtensionParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON202 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return clusterExtensionFromAPI(resp.JSON202), nil
}

// UpgradeClusterExtension upgrades an installed extension to the latest
// available version. The operation is asynchronous.
func (c *Client) UpgradeClusterExtension(ctx context.Context, clusterID, extensionID string) (*ClusterExtension, error) {
	resp, err := c.gen.UpgradeClusterExtensionWithResponse(ctx, cid(clusterID), uid(extensionID), &apiclient.UpgradeClusterExtensionParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON202 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return clusterExtensionFromAPI(resp.JSON202), nil
}

// UninstallExtension removes an extension from a cluster.
func (c *Client) UninstallExtension(ctx context.Context, clusterID, extensionID string) error {
	resp, err := c.gen.UninstallExtensionWithResponse(ctx, cid(clusterID), uid(extensionID), &apiclient.UninstallExtensionParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- Database exports ----

func nTime(n nullable.Nullable[time.Time]) string {
	if !n.IsSpecified() || n.IsNull() {
		return ""
	}
	v, err := n.Get()
	if err != nil {
		return ""
	}
	return fmtTime(v)
}

func nInt(n nullable.Nullable[int]) int64 {
	if !n.IsSpecified() || n.IsNull() {
		return 0
	}
	v, err := n.Get()
	if err != nil {
		return 0
	}
	return int64(v)
}

// Export is a database export job.
type Export struct {
	ID                 string
	ClusterID          string
	Format             string
	Status             string
	Progress           int64
	IncludeCredentials bool
	IsEncrypted        bool
	FileSizeBytes      int64
	Sha256Checksum     string
	DownloadURL        string
	ErrorMessage       string
	CreatedAt          string
	StartedAt          string
	CompletedAt        string
	ExpiresAt          string
}

func exportFromAPI(e *apiclient.Export) *Export {
	return &Export{
		ID: e.Id.String(), ClusterID: e.ClusterId.String(), Format: string(e.Format), Status: string(e.Status),
		Progress: int64(e.Progress), IncludeCredentials: e.IncludeCredentials, IsEncrypted: e.IsEncrypted,
		FileSizeBytes: nInt(e.FileSizeBytes), Sha256Checksum: nStr(e.Sha256Checksum), DownloadURL: nStr(e.DownloadUrl),
		ErrorMessage: nStr(e.ErrorMessage), CreatedAt: fmtTime(e.CreatedAt), StartedAt: nTime(e.StartedAt),
		CompletedAt: nTime(e.CompletedAt), ExpiresAt: nTime(e.ExpiresAt),
	}
}

// CreateExportRequest is the body for starting a database export.
type CreateExportRequest struct {
	Format             string
	IncludeCredentials bool
	EncryptionPassword string
}

// CreateExport starts a database export job (asynchronous).
func (c *Client) CreateExport(ctx context.Context, clusterID string, req CreateExportRequest) (*Export, error) {
	body := apiclient.CreateExportJSONRequestBody{Format: apiclient.ExportFormat(req.Format)}
	if req.IncludeCredentials {
		inc := true
		body.IncludeCredentials = &inc
	}
	if req.EncryptionPassword != "" {
		body.EncryptionPassword = &req.EncryptionPassword
	}
	resp, err := c.gen.CreateExportWithResponse(ctx, cid(clusterID), &apiclient.CreateExportParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON202 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return exportFromAPI(resp.JSON202), nil
}

// GetExport returns a single export job by ID.
func (c *Client) GetExport(ctx context.Context, clusterID, exportID string) (*Export, error) {
	resp, err := c.gen.GetExportWithResponse(ctx, cid(clusterID), uid(exportID), &apiclient.GetExportParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return exportFromAPI(resp.JSON200), nil
}

// ListExports returns the export jobs for a cluster.
func (c *Client) ListExports(ctx context.Context, clusterID string) ([]Export, error) {
	resp, err := c.gen.ListExportsWithResponse(ctx, cid(clusterID), &apiclient.ListExportsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]Export, 0, len(*resp.JSON200))
	for _, s := range *resp.JSON200 {
		out = append(out, Export{
			ID: s.Id.String(), ClusterID: s.ClusterId.String(), Format: string(s.Format), Status: string(s.Status),
			CreatedAt: fmtTime(s.CreatedAt), StartedAt: nTime(s.StartedAt), CompletedAt: nTime(s.CompletedAt), ExpiresAt: nTime(s.ExpiresAt),
		})
	}
	return out, nil
}

// DeleteExport removes an export archive.
func (c *Client) DeleteExport(ctx context.Context, clusterID, exportID string) error {
	resp, err := c.gen.DeleteExportWithResponse(ctx, cid(clusterID), uid(exportID), &apiclient.DeleteExportParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// WaitForExport polls an export job until it reaches a terminal state
// (completed/failed) or the context is cancelled.
func (c *Client) WaitForExport(ctx context.Context, clusterID, exportID string) (*Export, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		e, err := c.GetExport(ctx, clusterID, exportID)
		if err != nil {
			return nil, err
		}
		switch e.Status {
		case "completed":
			return e, nil
		case "failed":
			return e, fmt.Errorf("export %s failed: %s", exportID, e.ErrorMessage)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// ---- Theme & extension uploads (multipart) ----

// filePart writes a binary file part with an explicit Content-Type.
func filePart(mw *multipart.Writer, field, filename, contentType string, content []byte) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename))
	h.Set("Content-Type", contentType)
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = w.Write(content)
	return err
}

// jsonPart writes a JSON-encoded form part (Content-Type: application/json).
func jsonPart(mw *multipart.Writer, field string, v any) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q`, field))
	h.Set("Content-Type", "application/json")
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(v)
}

func themeFileContentType(filename string) string {
	if strings.HasSuffix(strings.ToLower(filename), ".jar") {
		return "application/java-archive"
	}
	return "application/zip"
}

// UploadThemeRequest is the body for uploading a theme archive.
type UploadThemeRequest struct {
	Name        string
	Description string
	Version     string
	ThemeTypes  []string
	FileName    string
	Content     []byte
}

// UploadTheme uploads a theme archive (ZIP or Keycloakify JAR) to a cluster.
func (c *Client) UploadTheme(ctx context.Context, clusterID string, req UploadThemeRequest) (*Theme, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", req.Name)
	if req.Description != "" {
		_ = mw.WriteField("description", req.Description)
	}
	if req.Version != "" {
		_ = mw.WriteField("version", req.Version)
	}
	for _, tt := range req.ThemeTypes {
		_ = mw.WriteField("theme_types", tt)
	}
	if err := filePart(mw, "theme_file", req.FileName, themeFileContentType(req.FileName), req.Content); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	resp, err := c.gen.UploadThemeWithBodyWithResponse(ctx, cid(clusterID), &apiclient.UploadThemeParams{APIVersion: c.ver()}, mw.FormDataContentType(), &buf)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	t := themeFromAPI(resp.JSON201)
	return &t, nil
}

// UpdateThemeMetadata updates a theme's name/description/version (no re-upload).
func (c *Client) UpdateThemeMetadata(ctx context.Context, clusterID, themeID, name, description, version string) (*Theme, error) {
	body := apiclient.UpdateThemeJSONRequestBody{}
	if name != "" {
		body.Name = &name
	}
	body.Description = strPtr(description)
	body.Version = strPtr(version)
	resp, err := c.gen.UpdateThemeWithResponse(ctx, cid(clusterID), uid(themeID), &apiclient.UpdateThemeParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	t := themeFromAPI(resp.JSON200)
	return &t, nil
}

// DeleteTheme removes a theme from a cluster.
func (c *Client) DeleteTheme(ctx context.Context, clusterID, themeID string) error {
	resp, err := c.gen.DeleteThemeWithResponse(ctx, cid(clusterID), uid(themeID), &apiclient.DeleteThemeParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ExtensionParameterOption is a dropdown choice for an extension parameter.
type ExtensionParameterOption struct {
	Label string
	Value string
}

// ExtensionParameterDef defines a single extension configuration parameter.
type ExtensionParameterDef struct {
	Key          string
	Label        string
	Type         string
	Required     bool
	DefaultValue string
	IsSensitive  bool
	Options      []ExtensionParameterOption
}

func toAPIParams(in []ExtensionParameterDef) []apiclient.ExtensionParameterInput {
	out := make([]apiclient.ExtensionParameterInput, 0, len(in))
	for _, p := range in {
		ip := apiclient.ExtensionParameterInput{
			Key: p.Key, Label: p.Label, Type: apiclient.ExtensionParameterFieldType(p.Type), Required: p.Required,
		}
		if p.DefaultValue != "" {
			ip.DefaultValue = strPtr(p.DefaultValue)
		}
		if p.IsSensitive {
			b := true
			ip.IsSensitive = &b
		}
		if len(p.Options) > 0 {
			opts := make([]apiclient.ExtensionParameterOption, 0, len(p.Options))
			for _, o := range p.Options {
				opts = append(opts, apiclient.ExtensionParameterOption{Label: o.Label, Value: o.Value})
			}
			ip.Options = &opts
		}
		out = append(out, ip)
	}
	return out
}

// UploadExtensionRequest is the body for uploading a custom extension JAR.
type UploadExtensionRequest struct {
	Name            string
	KeycloakVersion string
	Description     string
	IconURL         string
	RepositoryURL   string
	Version         string
	ParameterType   string
	Parameters      []ExtensionParameterDef
	JarFileName     string
	Jar             []byte
}

// UploadExtension uploads a custom extension JAR plus its metadata.
func (c *Client) UploadExtension(ctx context.Context, req UploadExtensionRequest) (*ExtensionInfo, error) {
	meta := apiclient.UploadExtensionMetadata{Name: req.Name, KeycloakVersion: req.KeycloakVersion}
	meta.Description = strPtr(req.Description)
	meta.IconUrl = strPtr(req.IconURL)
	meta.RepositoryUrl = strPtr(req.RepositoryURL)
	meta.Version = strPtr(req.Version)
	if req.ParameterType != "" {
		pt := apiclient.ExtensionParameterType(req.ParameterType)
		meta.ParameterType = &pt
	}
	if len(req.Parameters) > 0 {
		params := toAPIParams(req.Parameters)
		meta.Parameters = &params
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := filePart(mw, "jar", req.JarFileName, "application/java-archive", req.Jar); err != nil {
		return nil, err
	}
	if err := jsonPart(mw, "metadata", meta); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	resp, err := c.gen.UploadExtensionWithBodyWithResponse(ctx, &apiclient.UploadExtensionParams{APIVersion: c.ver()}, mw.FormDataContentType(), &buf)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	e := extensionInfoFromAPI(resp.JSON201)
	return &e, nil
}

// UpdateExtensionMetadata updates a custom extension's metadata and parameter
// schema (no JAR replacement).
func (c *Client) UpdateExtensionMetadata(ctx context.Context, extensionID string, req UploadExtensionRequest) (*ExtensionInfo, error) {
	body := apiclient.UpdateExtensionJSONRequestBody{}
	if req.Name != "" {
		body.Name = &req.Name
	}
	body.Description = nullableStr(req.Description)
	body.IconUrl = nullableStr(req.IconURL)
	body.RepositoryUrl = nullableStr(req.RepositoryURL)
	if req.ParameterType != "" {
		pt := apiclient.ExtensionParameterType(req.ParameterType)
		body.ParameterType = &pt
	}
	if len(req.Parameters) > 0 {
		params := toAPIParams(req.Parameters)
		body.Parameters = &params
	}
	resp, err := c.gen.UpdateExtensionWithResponse(ctx, uid(extensionID), &apiclient.UpdateExtensionParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	e := extensionInfoFromAPI(resp.JSON200)
	return &e, nil
}

// PublishExtensionVersion uploads a new JAR as a new version of an existing extension.
func (c *Client) PublishExtensionVersion(ctx context.Context, extensionID, version, jarFileName string, jar []byte) (*ExtensionInfo, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := filePart(mw, "jar", jarFileName, "application/java-archive", jar); err != nil {
		return nil, err
	}
	if err := jsonPart(mw, "metadata", apiclient.PublishExtensionVersionMetadata{Version: version}); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	resp, err := c.gen.PublishExtensionVersionWithBodyWithResponse(ctx, uid(extensionID), &apiclient.PublishExtensionVersionParams{APIVersion: c.ver()}, mw.FormDataContentType(), &buf)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	e := extensionInfoFromAPI(resp.JSON200)
	return &e, nil
}

// DeleteExtension removes a custom extension from the workspace catalog.
func (c *Client) DeleteExtension(ctx context.Context, extensionID string) error {
	resp, err := c.gen.DeleteExtensionWithResponse(ctx, uid(extensionID), &apiclient.DeleteExtensionParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// nullableStr maps "" to explicit null, otherwise to a present value. Used for
// PATCH bodies where omitting clears the field.
func nullableStr(s string) nullable.Nullable[string] {
	if s == "" {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(s)
}

// ---- Realm roles ----

// RealmRole mirrors a realm-scoped role.
type RealmRole struct {
	Name        string
	Description string
	Composite   bool
	ClientRole  bool
}

func realmRoleFromAPI(r *apiclient.RealmRole) RealmRole {
	out := RealmRole{Name: r.Name, Description: nStr(r.Description)}
	if r.Composite != nil {
		out.Composite = *r.Composite
	}
	if r.ClientRole != nil {
		out.ClientRole = *r.ClientRole
	}
	return out
}

// ListRealmRoles returns the realm-scoped roles of a realm.
func (c *Client) ListRealmRoles(ctx context.Context, clusterID, realm string) ([]RealmRole, error) {
	resp, err := c.gen.ListRealmRolesWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.ListRealmRolesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]RealmRole, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, realmRoleFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetRealmRole returns a single realm role by name.
func (c *Client) GetRealmRole(ctx context.Context, clusterID, realm, name string) (*RealmRole, error) {
	resp, err := c.gen.GetRealmRoleWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), name, &apiclient.GetRealmRoleParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	r := realmRoleFromAPI(resp.JSON200)
	return &r, nil
}

// CreateRealmRole creates a realm role.
func (c *Client) CreateRealmRole(ctx context.Context, clusterID, realm, name, description string) (*RealmRole, error) {
	body := apiclient.CreateRealmRoleJSONRequestBody{Name: name}
	body.Description = strPtr(description)
	resp, err := c.gen.CreateRealmRoleWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.CreateRealmRoleParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	r := realmRoleFromAPI(resp.JSON201)
	return &r, nil
}

// UpdateRealmRole updates a realm role's description (and optionally renames it).
func (c *Client) UpdateRealmRole(ctx context.Context, clusterID, realm, name, description string) (*RealmRole, error) {
	body := apiclient.UpdateRealmRoleJSONRequestBody{}
	body.Description = strPtr(description)
	resp, err := c.gen.UpdateRealmRoleWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), name, &apiclient.UpdateRealmRoleParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	r := realmRoleFromAPI(resp.JSON200)
	return &r, nil
}

// DeleteRealmRole removes a realm role.
func (c *Client) DeleteRealmRole(ctx context.Context, clusterID, realm, name string) error {
	resp, err := c.gen.DeleteRealmRoleWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), name, &apiclient.DeleteRealmRoleParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- Realm groups ----

// RealmGroup mirrors a realm group.
type RealmGroup struct {
	ID   string
	Name string
	Path string
}

func realmGroupFromAPI(g *apiclient.RealmGroup) RealmGroup {
	return RealmGroup{ID: g.Id.String(), Name: g.Name, Path: g.Path}
}

// ListRealmGroups returns the top-level groups of a realm.
func (c *Client) ListRealmGroups(ctx context.Context, clusterID, realm string) ([]RealmGroup, error) {
	resp, err := c.gen.ListRealmGroupsWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.ListRealmGroupsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]RealmGroup, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, realmGroupFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetRealmGroup returns a single realm group by ID.
func (c *Client) GetRealmGroup(ctx context.Context, clusterID, realm, groupID string) (*RealmGroup, error) {
	resp, err := c.gen.GetRealmGroupWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), uid(groupID), &apiclient.GetRealmGroupParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	g := realmGroupFromAPI(resp.JSON200)
	return &g, nil
}

// CreateRealmGroup creates a realm group, optionally nested under a parent.
func (c *Client) CreateRealmGroup(ctx context.Context, clusterID, realm, name, parentID string) (*RealmGroup, error) {
	body := apiclient.CreateRealmGroupJSONRequestBody{Name: name}
	if parentID != "" {
		pid := uid(parentID)
		body.ParentId = &pid
	}
	resp, err := c.gen.CreateRealmGroupWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.CreateRealmGroupParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	g := realmGroupFromAPI(resp.JSON201)
	return &g, nil
}

// UpdateRealmGroup renames a realm group.
func (c *Client) UpdateRealmGroup(ctx context.Context, clusterID, realm, groupID, name string) (*RealmGroup, error) {
	body := apiclient.UpdateRealmGroupJSONRequestBody{Name: &name}
	resp, err := c.gen.UpdateRealmGroupWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), uid(groupID), &apiclient.UpdateRealmGroupParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	g := realmGroupFromAPI(resp.JSON200)
	return &g, nil
}

// DeleteRealmGroup removes a realm group.
func (c *Client) DeleteRealmGroup(ctx context.Context, clusterID, realm, groupID string) error {
	resp, err := c.gen.DeleteRealmGroupWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), uid(groupID), &apiclient.DeleteRealmGroupParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- Realm users ----

// RealmUser mirrors a realm user.
type RealmUser struct {
	ID            string
	Username      string
	Email         string
	FirstName     string
	LastName      string
	Enabled       bool
	EmailVerified bool
	CreatedAt     string
	LastLoginAt   string
}

func realmUserFromAPI(u *apiclient.RealmUser) RealmUser {
	out := RealmUser{
		ID: u.Id, Username: string(u.Username), Email: string(u.Email), Enabled: u.Enabled,
	}
	out.FirstName = strDerefPtr(u.FirstName)
	out.LastName = strDerefPtr(u.LastName)
	if u.EmailVerified != nil {
		out.EmailVerified = *u.EmailVerified
	}
	if u.CreatedAt != nil {
		out.CreatedAt = fmtTime(*u.CreatedAt)
	}
	if u.LastLoginAt != nil {
		out.LastLoginAt = fmtTime(*u.LastLoginAt)
	}
	return out
}

func strDerefPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// CreateRealmUserRequest is the body for creating a realm user.
type CreateRealmUserRequest struct {
	Username          string
	Email             string
	FirstName         string
	LastName          string
	Enabled           bool
	TemporaryPassword string
}

// ListRealmUsers returns the users of a realm.
func (c *Client) ListRealmUsers(ctx context.Context, clusterID, realm string) ([]RealmUser, error) {
	resp, err := c.gen.ListRealmUsersWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.ListRealmUsersParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]RealmUser, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, realmUserFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetRealmUser returns a single realm user by ID.
func (c *Client) GetRealmUser(ctx context.Context, clusterID, realm, userID string) (*RealmUser, error) {
	resp, err := c.gen.GetRealmUserWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), userID, &apiclient.GetRealmUserParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	u := realmUserFromAPI(resp.JSON200)
	return &u, nil
}

// CreateRealmUser creates a realm user.
func (c *Client) CreateRealmUser(ctx context.Context, clusterID, realm string, req CreateRealmUserRequest) (*RealmUser, error) {
	enabled := req.Enabled
	body := apiclient.CreateRealmUserJSONRequestBody{
		Username: req.Username, Email: openapitypes.Email(req.Email), TemporaryPassword: req.TemporaryPassword, Enabled: &enabled,
	}
	body.FirstName = strPtr(req.FirstName)
	body.LastName = strPtr(req.LastName)
	resp, err := c.gen.CreateRealmUserWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), &apiclient.CreateRealmUserParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	u := realmUserFromAPI(resp.JSON201)
	return &u, nil
}

// UpdateRealmUser updates a realm user's profile fields.
func (c *Client) UpdateRealmUser(ctx context.Context, clusterID, realm, userID string, req CreateRealmUserRequest, emailVerified bool) (*RealmUser, error) {
	enabled := req.Enabled
	ev := emailVerified
	email := openapitypes.Email(req.Email)
	body := apiclient.UpdateRealmUserJSONRequestBody{Enabled: &enabled, EmailVerified: &ev, Email: &email}
	body.FirstName = strPtr(req.FirstName)
	body.LastName = strPtr(req.LastName)
	resp, err := c.gen.UpdateRealmUserWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), userID, &apiclient.UpdateRealmUserParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	u := realmUserFromAPI(resp.JSON200)
	return &u, nil
}

// DeleteRealmUser removes a realm user.
func (c *Client) DeleteRealmUser(ctx context.Context, clusterID, realm, userID string) error {
	resp, err := c.gen.DeleteRealmUserWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), userID, &apiclient.DeleteRealmUserParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- Realm user role & group assignments ----

// ListRealmUserRoles returns the realm roles assigned to a user.
func (c *Client) ListRealmUserRoles(ctx context.Context, clusterID, realm, userID string) ([]RealmRole, error) {
	resp, err := c.gen.ListRealmUserRolesWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), userID, &apiclient.ListRealmUserRolesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]RealmRole, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, realmRoleFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// AssignRealmUserRole assigns a single realm role to a user.
func (c *Client) AssignRealmUserRole(ctx context.Context, clusterID, realm, userID, roleName string) error {
	body := apiclient.AssignRealmUserRolesJSONRequestBody{RoleNames: []apiclient.RealmRoleName{roleName}}
	resp, err := c.gen.AssignRealmUserRolesWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), userID, &apiclient.AssignRealmUserRolesParams{APIVersion: c.ver()}, body)
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// RemoveRealmUserRole removes a realm role from a user.
func (c *Client) RemoveRealmUserRole(ctx context.Context, clusterID, realm, userID, roleName string) error {
	resp, err := c.gen.RemoveRealmUserRoleWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), userID, roleName, &apiclient.RemoveRealmUserRoleParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ListRealmUserGroups returns the groups a user belongs to.
func (c *Client) ListRealmUserGroups(ctx context.Context, clusterID, realm, userID string) ([]RealmGroup, error) {
	resp, err := c.gen.ListRealmUserGroupsWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), userID, &apiclient.ListRealmUserGroupsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]RealmGroup, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, realmGroupFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// AddRealmUserToGroup adds a user to a group.
func (c *Client) AddRealmUserToGroup(ctx context.Context, clusterID, realm, userID, groupID string) error {
	resp, err := c.gen.AddRealmUserToGroupWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), userID, uid(groupID), &apiclient.AddRealmUserToGroupParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// RemoveRealmUserFromGroup removes a user from a group.
func (c *Client) RemoveRealmUserFromGroup(ctx context.Context, clusterID, realm, userID, groupID string) error {
	resp, err := c.gen.RemoveRealmUserFromGroupWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), userID, uid(groupID), &apiclient.RemoveRealmUserFromGroupParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ---- Application roles & sessions ----

// ApplicationRole is a role assigned to an application's service account.
type ApplicationRole struct {
	Name        string
	Description string
	Composite   bool
	ClientRole  bool
}

// ListApplicationRoles returns the roles assigned to an application's service account.
func (c *Client) ListApplicationRoles(ctx context.Context, clusterID, realm, clientID string) ([]ApplicationRole, error) {
	resp, err := c.gen.ListApplicationRolesWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), &apiclient.ListApplicationRolesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]ApplicationRole, 0, len(*resp.JSON200))
	for _, r := range *resp.JSON200 {
		out = append(out, ApplicationRole{Name: r.Name, Description: strDerefPtr(r.Description), Composite: r.Composite, ClientRole: r.ClientRole})
	}
	return out, nil
}

// AssignApplicationRole assigns a role to an application's service account. Pass
// roleClientID for a client role, or "" for a realm role.
func (c *Client) AssignApplicationRole(ctx context.Context, clusterID, realm, clientID, roleName, roleClientID string) error {
	body := apiclient.AssignApplicationRoleJSONRequestBody{Name: roleName}
	if roleClientID != "" {
		body.RoleClientId = &roleClientID
	}
	resp, err := c.gen.AssignApplicationRoleWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), &apiclient.AssignApplicationRoleParams{APIVersion: c.ver()}, body)
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// RemoveApplicationRole removes a role from an application's service account.
func (c *Client) RemoveApplicationRole(ctx context.Context, clusterID, realm, clientID, roleName, roleClientID string) error {
	params := &apiclient.RemoveApplicationRoleParams{APIVersion: c.ver()}
	if roleClientID != "" {
		params.RoleClientId = &roleClientID
	}
	resp, err := c.gen.RemoveApplicationRoleWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), roleName, params)
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// ApplicationSession is an active user session for an application.
type ApplicationSession struct {
	ID           string
	UserID       string
	Username     string
	Email        string
	IPAddress    string
	StartedAt    string
	LastAccessAt string
}

// ListApplicationSessions returns active user sessions for an application.
func (c *Client) ListApplicationSessions(ctx context.Context, clusterID, realm, clientID string) ([]ApplicationSession, error) {
	resp, err := c.gen.ListApplicationSessionsWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), &apiclient.ListApplicationSessionsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]ApplicationSession, 0, len(*resp.JSON200))
	for _, s := range *resp.JSON200 {
		out = append(out, ApplicationSession{
			ID: s.Id, UserID: s.UserId, Username: s.Username, Email: strDerefPtr(s.Email), IPAddress: strDerefPtr(s.IpAddress),
			StartedAt: fmtTime(s.StartedAt), LastAccessAt: fmtTime(s.LastAccessAt),
		})
	}
	return out, nil
}

// ---- Read-only metadata (versions, templates, builds, upgrades, routes) ----

// ClusterTypeVersions returns the Keycloak versions available for a cluster type.
func (c *Client) ClusterTypeVersions(ctx context.Context, clusterType string) ([]string, error) {
	resp, err := c.gen.GetClusterTypeVersionsWithResponse(ctx, apiclient.ClusterType(clusterType), &apiclient.GetClusterTypeVersionsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return *resp.JSON200, nil
}

// ProviderTemplate is a pre-configured identity-provider template.
type ProviderTemplate struct {
	ID          string
	Name        string
	Description string
	Type        string
}

// ListIdentityProviderTemplates returns the identity-provider template catalog.
func (c *Client) ListIdentityProviderTemplates(ctx context.Context) ([]ProviderTemplate, error) {
	resp, err := c.gen.ListIdentityProviderTemplatesWithResponse(ctx, &apiclient.ListIdentityProviderTemplatesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]ProviderTemplate, 0, len(*resp.JSON200))
	for _, t := range *resp.JSON200 {
		out = append(out, ProviderTemplate{ID: t.Id, Name: t.Name, Description: t.Description, Type: string(t.Type)})
	}
	return out, nil
}

// ListDomainRoutes returns the routes configured on a custom domain.
func (c *Client) ListDomainRoutes(ctx context.Context, clusterID, domainID string) ([]DomainRoute, error) {
	resp, err := c.gen.ListDomainRoutesWithResponse(ctx, cid(clusterID), uid(domainID), &apiclient.ListDomainRoutesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]DomainRoute, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, *domainRouteFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// ClusterUpgrade is a cluster version-upgrade record.
type ClusterUpgrade struct {
	ID          string
	FromVersion string
	ToVersion   string
	Phase       string
	StartedAt   string
	CompletedAt string
}

// ListClusterUpgrades returns the upgrade history for a cluster.
func (c *Client) ListClusterUpgrades(ctx context.Context, clusterID string) ([]ClusterUpgrade, error) {
	resp, err := c.gen.ListClusterUpgradesWithResponse(ctx, cid(clusterID), &apiclient.ListClusterUpgradesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]ClusterUpgrade, 0, len(*resp.JSON200))
	for _, u := range *resp.JSON200 {
		out = append(out, ClusterUpgrade{
			ID: u.Id, FromVersion: string(u.FromVersion), ToVersion: string(u.ToVersion), Phase: u.Phase,
			StartedAt: fmtTime(u.StartedAt), CompletedAt: nTime(u.CompletedAt),
		})
	}
	return out, nil
}

// ---- OIDC discovery ----

// OIDCDiscovery is the subset of an OIDC discovery document used to pre-fill an
// identity provider.
type OIDCDiscovery struct {
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserinfoEndpoint      string
	JwksURI               string
	ScopesSupported       []string
}

// DiscoverOIDC fetches the OIDC discovery document for an issuer URL.
func (c *Client) DiscoverOIDC(ctx context.Context, issuerURL string) (*OIDCDiscovery, error) {
	resp, err := c.gen.DiscoverOIDCWithResponse(ctx, &apiclient.DiscoverOIDCParams{APIVersion: c.ver()}, apiclient.DiscoverOIDCJSONRequestBody{IssuerUrl: issuerURL})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	d := resp.JSON200
	out := &OIDCDiscovery{Issuer: d.Issuer, AuthorizationEndpoint: d.AuthorizationEndpoint, TokenEndpoint: d.TokenEndpoint}
	out.UserinfoEndpoint = strDerefPtr(d.UserinfoEndpoint)
	out.JwksURI = strDerefPtr(d.JwksUri)
	if d.ScopesSupported != nil {
		out.ScopesSupported = *d.ScopesSupported
	}
	return out, nil
}

// ---- Insights, credentials, group members, upgrade path ----

// ClusterInsights returns a cluster analytics document as raw JSON. kind is one
// of: overview, authentication, events, performance, security.
func (c *Client) ClusterInsights(ctx context.Context, clusterID, kind string) ([]byte, error) {
	id := cid(clusterID)
	var raw []byte
	var resp *http.Response
	var err error
	switch kind {
	case "authentication":
		r, e := c.gen.GetClusterInsightsAuthenticationWithResponse(ctx, id, &apiclient.GetClusterInsightsAuthenticationParams{APIVersion: c.ver()})
		err = e
		if r != nil {
			raw, resp = r.Body, r.HTTPResponse
		}
	case "events":
		r, e := c.gen.GetClusterInsightsEventsWithResponse(ctx, id, &apiclient.GetClusterInsightsEventsParams{APIVersion: c.ver()})
		err = e
		if r != nil {
			raw, resp = r.Body, r.HTTPResponse
		}
	case "performance":
		r, e := c.gen.GetClusterInsightsPerformanceWithResponse(ctx, id, &apiclient.GetClusterInsightsPerformanceParams{APIVersion: c.ver()})
		err = e
		if r != nil {
			raw, resp = r.Body, r.HTTPResponse
		}
	case "security":
		r, e := c.gen.GetClusterInsightsSecurityWithResponse(ctx, id, &apiclient.GetClusterInsightsSecurityParams{APIVersion: c.ver()})
		err = e
		if r != nil {
			raw, resp = r.Body, r.HTTPResponse
		}
	default: // overview
		r, e := c.gen.GetClusterInsightsOverviewWithResponse(ctx, id, &apiclient.GetClusterInsightsOverviewParams{APIVersion: c.ver()})
		err = e
		if r != nil {
			raw, resp = r.Body, r.HTTPResponse
		}
	}
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, statusError(resp, raw)
	}
	return raw, nil
}

// ClusterCredentials holds the cluster automation service account credentials
// for the OAuth2 client_credentials grant.
type ClusterCredentials struct {
	ClientID     string
	ClientSecret string
	TokenURL     string
}

// GetClusterCredentials returns a cluster's automation service account credentials.
func (c *Client) GetClusterCredentials(ctx context.Context, clusterID string) (*ClusterCredentials, error) {
	resp, err := c.gen.GetClusterCredentialsWithResponse(ctx, cid(clusterID), &apiclient.GetClusterCredentialsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return &ClusterCredentials{
		ClientID:     resp.JSON200.ClientId,
		ClientSecret: resp.JSON200.ClientSecret,
		TokenURL:     resp.JSON200.TokenUrl,
	}, nil
}

// ListRealmGroupMembers returns the users that belong to a group.
func (c *Client) ListRealmGroupMembers(ctx context.Context, clusterID, realm, groupID string) ([]RealmUser, error) {
	resp, err := c.gen.ListRealmGroupMembersWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), uid(groupID), &apiclient.ListRealmGroupMembersParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]RealmUser, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, realmUserFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// UpgradePathStep is one step in a cluster's recommended upgrade path.
type UpgradePathStep struct {
	Version  string
	Required bool
}

// GetClusterUpgradePath returns the recommended version-upgrade path for a cluster.
func (c *Client) GetClusterUpgradePath(ctx context.Context, clusterID string) ([]UpgradePathStep, error) {
	resp, err := c.gen.GetClusterUpgradePathWithResponse(ctx, cid(clusterID), &apiclient.GetClusterUpgradePathParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]UpgradePathStep, 0, len(*resp.JSON200))
	for _, s := range *resp.JSON200 {
		out = append(out, UpgradePathStep{Version: string(s.Version), Required: s.Required})
	}
	return out, nil
}

// ---- Cluster logs, security logs, events, single build ----

// LogEntry is one cluster log line.
type LogEntry struct {
	Timestamp string
	Level     string
	Category  string
	Message   string
	Source    string
}

// LogQuery filters a log query.
type LogQuery struct {
	Limit  int
	Level  string
	Search string
}

// ListClusterLogs returns cluster application logs.
func (c *Client) ListClusterLogs(ctx context.Context, clusterID string, q LogQuery) ([]LogEntry, error) {
	params := &apiclient.ListClusterLogsParams{APIVersion: c.ver()}
	if q.Limit > 0 {
		l := apiclient.LogPageLimit(q.Limit)
		params.Limit = &l
	}
	if q.Level != "" {
		lv := apiclient.LogLevel(q.Level)
		params.Level = &lv
	}
	if q.Search != "" {
		params.Search = &q.Search
	}
	resp, err := c.gen.ListClusterLogsWithResponse(ctx, cid(clusterID), params)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]LogEntry, 0, len(*resp.JSON200))
	for _, l := range *resp.JSON200 {
		out = append(out, LogEntry{Timestamp: fmtTime(l.Timestamp), Level: string(l.Level), Category: l.Category, Message: l.Message, Source: l.Source})
	}
	return out, nil
}

// SecurityLogEntry is one cluster security-log line.
type SecurityLogEntry struct {
	Timestamp string
	Type      string
	Action    string
	SourceIP  string
	Country   string
	Method    string
	URI       string
	Message   string
}

// ListClusterSecurityLogs returns cluster edge-security logs (WAF, geo, etc.).
func (c *Client) ListClusterSecurityLogs(ctx context.Context, clusterID string, limit int, search string) ([]SecurityLogEntry, error) {
	params := &apiclient.ListClusterSecurityLogsParams{APIVersion: c.ver()}
	if limit > 0 {
		l := apiclient.LogPageLimit(limit)
		params.Limit = &l
	}
	if search != "" {
		params.Search = &search
	}
	resp, err := c.gen.ListClusterSecurityLogsWithResponse(ctx, cid(clusterID), params)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]SecurityLogEntry, 0, len(*resp.JSON200))
	for _, s := range *resp.JSON200 {
		out = append(out, SecurityLogEntry{
			Timestamp: fmtTime(s.Timestamp), Type: string(s.Type), Action: string(s.Action),
			SourceIP: s.SourceIp, Country: nStr(s.Country), Method: s.Method, URI: s.Uri, Message: s.Message,
		})
	}
	return out, nil
}

// EventEntry is one Keycloak event.
type EventEntry struct {
	Timestamp string
	Category  string
	Type      string
	RealmName string
	ClientID  string
	Username  string
	IPAddress string
	Error     string
}

// EventQuery filters an events query.
type EventQuery struct {
	Limit    int
	Category string
	Realm    string
	Username string
	Search   string
}

// ListClusterEvents returns Keycloak admin/user events.
func (c *Client) ListClusterEvents(ctx context.Context, clusterID string, q EventQuery) ([]EventEntry, error) {
	params := &apiclient.ListClusterEventsParams{APIVersion: c.ver()}
	if q.Limit > 0 {
		l := apiclient.PageLimit(q.Limit)
		params.Limit = &l
	}
	if q.Category != "" {
		cat := apiclient.EventCategory(q.Category)
		params.Category = &cat
	}
	if q.Realm != "" {
		rn := apiclient.RealmName(q.Realm)
		params.Realm = &rn
	}
	if q.Username != "" {
		params.Username = &q.Username
	}
	if q.Search != "" {
		params.Search = &q.Search
	}
	resp, err := c.gen.ListClusterEventsWithResponse(ctx, cid(clusterID), params)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]EventEntry, 0, len(*resp.JSON200))
	for _, e := range *resp.JSON200 {
		typ := strDerefPtr((*string)(e.Type))
		if typ == "" && e.OperationType != nil {
			typ = string(*e.OperationType)
		}
		out = append(out, EventEntry{
			Timestamp: fmtTime(e.Timestamp), Category: string(e.Category), Type: typ,
			RealmName: e.RealmName, ClientID: strDerefPtr(e.ClientId), Username: strDerefPtr(e.Username),
			IPAddress: strDerefPtr(e.IpAddress), Error: strDerefPtr(e.Error),
		})
	}
	return out, nil
}

// ExportClusterEvents exports a cluster's events as a raw document (CSV/JSON).
func (c *Client) ExportClusterEvents(ctx context.Context, clusterID string) ([]byte, error) {
	resp, err := c.gen.ExportClusterEventsWithResponse(ctx, cid(clusterID), &apiclient.ExportClusterEventsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return resp.Body, nil
}
