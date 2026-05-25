// Package skycloak is a thin, ergonomic facade over the generated Skycloak API
// client (internal/apiclient). It exposes domain structs and methods the
// provider's resources consume, mapping them to/from the generated wire types.
// The HTTP layer and wire types are generated from the OpenAPI spec, so they
// stay in sync with the API on `make generate`.
package skycloak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

const defaultEndpoint = "https://api.skycloak.io"

// Client wraps the generated API client.
type Client struct {
	gen *apiclient.ClientWithResponses
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
	// Retry 429/5xx with Retry-After-aware backoff.
	cfg.httpClient.Transport = &retryTransport{base: cfg.httpClient.Transport, maxRetries: 4}

	editor := func(_ context.Context, req *http.Request) error {
		req.Header.Set("apikey", apiKey)
		req.Header.Set("Accept", "application/json")
		if apiVersion != "" {
			req.Header.Set("API-Version", apiVersion)
		}
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
	return &Client{gen: gen}
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
	ID        string
	Name      string
	Type      string
	Size      string
	Version   string
	Location  string
	Status    string
	URL       string
	CreatedAt string
	UpdatedAt string
}

// CreateClusterRequest is the body for creating a cluster.
type CreateClusterRequest struct {
	Name     string
	Type     string
	Size     string
	Version  string
	Location string
}

// UpdateClusterRequest holds the mutable cluster fields.
type UpdateClusterRequest struct {
	Size    *string
	Version *string
}

func clusterFromAPI(c *apiclient.Cluster) *Cluster {
	return &Cluster{
		ID: c.Id.String(), Name: string(c.Name), Type: string(c.Type), Size: string(c.Size),
		Version: string(c.Version), Location: string(c.Location), Status: string(c.Status),
		URL: c.Url, CreatedAt: fmtTime(c.CreatedAt), UpdatedAt: fmtTime(c.UpdatedAt),
	}
}

// ListClusters returns the workspace's clusters.
func (c *Client) ListClusters(ctx context.Context) ([]Cluster, error) {
	resp, err := c.gen.ListClustersWithResponse(ctx)
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
			URL: s.Url, CreatedAt: fmtTime(s.CreatedAt), UpdatedAt: fmtTime(s.UpdatedAt),
		})
	}
	return out, nil
}

// GetCluster returns a single cluster by ID.
func (c *Client) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	resp, err := c.gen.GetClusterWithResponse(ctx, cid(id))
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
		Name:     apiclient.ClusterName(req.Name),
		Size:     apiclient.ClusterSize(req.Size),
		Version:  apiclient.KeycloakVersion(req.Version),
		Location: apiclient.ClusterLocation(req.Location),
	}
	if req.Type != "" {
		t := apiclient.ClusterType(req.Type)
		body.Type = &t
	}
	resp, err := c.gen.CreateClusterWithResponse(ctx, body)
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
	body := apiclient.UpdateClusterJSONRequestBody{}
	if req.Size != nil {
		s := apiclient.ClusterSize(*req.Size)
		body.Size = &s
	}
	if req.Version != nil {
		v := apiclient.KeycloakVersion(*req.Version)
		body.Version = &v
	}
	resp, err := c.gen.UpdateClusterWithResponse(ctx, cid(id), body)
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
	resp, err := c.gen.DeleteClusterWithResponse(ctx, cid(id))
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
	resp, err := c.gen.ListRealmsWithResponse(ctx, cid(clusterID))
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
	resp, err := c.gen.GetRealmWithResponse(ctx, cid(clusterID), apiclient.RealmName(name))
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
	resp, err := c.gen.CreateRealmWithResponse(ctx, cid(clusterID), body)
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
	resp, err := c.gen.UpdateRealmWithResponse(ctx, cid(clusterID), apiclient.RealmName(name), body)
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
	resp, err := c.gen.DeleteRealmWithResponse(ctx, cid(clusterID), apiclient.RealmName(name))
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

// ListApplications returns the applications in a realm.
func (c *Client) ListApplications(ctx context.Context, clusterID, realm string) ([]Application, error) {
	resp, err := c.gen.ListApplicationsWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), nil)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]Application, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, *applicationFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetApplication returns a single application by client ID.
func (c *Client) GetApplication(ctx context.Context, clusterID, realm, clientID string) (*Application, error) {
	resp, err := c.gen.GetApplicationWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID))
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
	resp, err := c.gen.CreateApplicationWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), body)
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
	resp, err := c.gen.UpdateApplicationWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID), body)
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
	resp, err := c.gen.DeleteApplicationWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ApplicationClientId(clientID))
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
	resp, err := c.gen.GetSmtpConfigWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm))
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

	resp, err := c.gen.UpsertSmtpConfigWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), body)
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
	resp, err := c.gen.DeleteSmtpConfigWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm))
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
	resp, err := c.gen.ListClusterLocationsWithResponse(ctx)
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
	resp, err := c.gen.ListClusterTypesWithResponse(ctx)
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
	resp, err := c.gen.ListClusterFeaturesWithResponse(ctx)
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
	resp, err := c.gen.ListIdentityProvidersWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm))
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
	resp, err := c.gen.GetIdentityProviderWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ProviderId(providerID))
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
	resp, err := c.gen.CreateIdentityProviderWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), body)
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
	resp, err := c.gen.UpdateIdentityProviderWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ProviderId(providerID), body)
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
	resp, err := c.gen.DeleteIdentityProviderWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), apiclient.ProviderId(providerID))
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}
