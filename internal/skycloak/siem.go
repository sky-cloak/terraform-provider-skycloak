package skycloak

import (
	"context"

	"github.com/google/uuid"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// SIEMSource selects the workspace data stream forwarded to a destination.
type SIEMSource struct {
	Type               string
	ClusterIDs         []string
	Realms             []string
	KeycloakEventTypes []string
}

// SIEMBatch tunes forwarding batch size and flush interval.
type SIEMBatch struct {
	MaxEvents          int64
	MaxIntervalSeconds int64
}

// SIEMSyslog is the syslog destination config.
type SIEMSyslog struct {
	Host     string
	Port     int64
	Protocol string
	Format   string
}

// SIEMS3 is the S3 destination config. AccessKeyID/SecretAccessKey are
// write-only; reads report HasAccessKeySecret instead.
type SIEMS3 struct {
	Bucket             string
	Region             string
	Prefix             string
	AuthType           string
	AccessKeyID        string
	SecretAccessKey    string
	RoleArn            string
	ExternalID         string
	HasAccessKeySecret bool
}

// SIEMHTTP is the HTTP destination config. BearerToken/Password/Headers values
// are write-only; reads report HasAuthCredentials and header names only.
type SIEMHTTP struct {
	URL                string
	AuthType           string
	Username           string
	Password           string
	BearerToken        string
	Headers            map[string]string
	HasAuthCredentials bool
	HeaderNames        []string
}

// SIEMDestination is a saved SIEM forwarding destination for a workspace.
type SIEMDestination struct {
	ID              string
	Name            string
	Enabled         bool
	Type            string
	Source          SIEMSource
	Batch           *SIEMBatch
	Syslog          *SIEMSyslog
	S3              *SIEMS3
	HTTP            *SIEMHTTP
	HealthStatus    string
	FailureCount    int64
	LastError       string
	LastSentAt      string
	TotalEventsSent int64
	TotalLogsSent   int64
	TotalBytesSent  int64
	CreatedAt       string
	UpdatedAt       string
}

// SIEMTestResult is the outcome of a destination connectivity test.
type SIEMTestResult struct {
	Success bool
	Message string
}

// CreateSIEMDestinationRequest holds the destination definition. Secret fields
// are write-only.
type CreateSIEMDestinationRequest struct {
	Name    string
	Enabled *bool
	Type    string
	Source  SIEMSource
	Batch   *SIEMBatch
	Syslog  *SIEMSyslog
	S3      *SIEMS3
	HTTP    *SIEMHTTP
}

func did(s string) apiclient.SIEMDestinationId {
	id, _ := uuid.Parse(s)
	return id
}

func siemSourceToAPI(s SIEMSource) apiclient.SIEMSourceConfig {
	out := apiclient.SIEMSourceConfig{Type: apiclient.SIEMSourceType(s.Type)}
	if len(s.ClusterIDs) > 0 {
		ids := make([]apiclient.ClusterId, 0, len(s.ClusterIDs))
		for _, c := range s.ClusterIDs {
			ids = append(ids, cid(c))
		}
		out.ClusterIds = &ids
	}
	if len(s.Realms) > 0 {
		realms := make([]apiclient.RealmName, 0, len(s.Realms))
		for _, r := range s.Realms {
			realms = append(realms, apiclient.RealmName(r))
		}
		out.Realms = &realms
	}
	if len(s.KeycloakEventTypes) > 0 {
		evts := s.KeycloakEventTypes
		out.KeycloakEventTypes = &evts
	}
	return out
}

func siemBatchToAPI(b *SIEMBatch) *apiclient.SIEMBatchConfig {
	if b == nil {
		return nil
	}
	out := &apiclient.SIEMBatchConfig{}
	if b.MaxEvents > 0 {
		v := int(b.MaxEvents)
		out.MaxEvents = &v
	}
	if b.MaxIntervalSeconds > 0 {
		v := int(b.MaxIntervalSeconds)
		out.MaxIntervalSeconds = &v
	}
	return out
}

func siemSyslogToAPI(s *SIEMSyslog) *apiclient.SIEMSyslogConfig {
	if s == nil {
		return nil
	}
	return &apiclient.SIEMSyslogConfig{
		Host: s.Host, Port: int(s.Port),
		Protocol: apiclient.SyslogProtocol(s.Protocol), Format: apiclient.SyslogFormat(s.Format),
	}
}

func siemS3ToAPI(s *SIEMS3) *apiclient.SIEMS3Config {
	if s == nil {
		return nil
	}
	out := &apiclient.SIEMS3Config{
		Bucket: s.Bucket, Region: s.Region, AuthType: apiclient.S3AuthType(s.AuthType),
		Prefix: strPtr(s.Prefix), RoleArn: strPtr(s.RoleArn), ExternalId: strPtr(s.ExternalID),
	}
	out.AccessKeyId = strPtr(s.AccessKeyID)
	out.SecretAccessKey = strPtr(s.SecretAccessKey)
	return out
}

func siemHTTPToAPI(h *SIEMHTTP) *apiclient.SIEMHTTPConfig {
	if h == nil {
		return nil
	}
	out := &apiclient.SIEMHTTPConfig{Url: h.URL, AuthType: apiclient.HTTPAuthType(h.AuthType)}
	out.Username = strPtr(h.Username)
	out.Password = strPtr(h.Password)
	out.BearerToken = strPtr(h.BearerToken)
	if len(h.Headers) > 0 {
		hd := h.Headers
		out.Headers = &hd
	}
	return out
}

func siemFromAPI(d *apiclient.SIEMDestination) *SIEMDestination {
	out := &SIEMDestination{
		ID: d.Id.String(), Name: string(d.Name), Enabled: d.Enabled, Type: string(d.Type),
		HealthStatus: string(d.HealthStatus), FailureCount: int64(d.FailureCount),
		LastError: strDerefPtr(d.LastError), TotalEventsSent: d.TotalEventsSent,
		TotalLogsSent: d.TotalLogsSent, TotalBytesSent: d.TotalBytesSent,
		CreatedAt: fmtTime(d.CreatedAt), UpdatedAt: fmtTime(d.UpdatedAt),
	}
	if d.LastSentAt != nil {
		out.LastSentAt = fmtTime(*d.LastSentAt)
	}
	out.Source = SIEMSource{Type: string(d.Source.Type), KeycloakEventTypes: derefSlice(d.Source.KeycloakEventTypes)}
	if d.Source.ClusterIds != nil {
		for _, c := range *d.Source.ClusterIds {
			out.Source.ClusterIDs = append(out.Source.ClusterIDs, c.String())
		}
	}
	if d.Source.Realms != nil {
		for _, r := range *d.Source.Realms {
			out.Source.Realms = append(out.Source.Realms, string(r))
		}
	}
	if d.Batch.MaxEvents != nil || d.Batch.MaxIntervalSeconds != nil {
		out.Batch = &SIEMBatch{MaxEvents: derefIntStar(d.Batch.MaxEvents), MaxIntervalSeconds: derefIntStar(d.Batch.MaxIntervalSeconds)}
	}
	if d.Syslog != nil {
		out.Syslog = &SIEMSyslog{Host: d.Syslog.Host, Port: int64(d.Syslog.Port), Protocol: string(d.Syslog.Protocol), Format: string(d.Syslog.Format)}
	}
	if d.S3 != nil {
		out.S3 = &SIEMS3{
			Bucket: d.S3.Bucket, Region: d.S3.Region, Prefix: strDerefPtr(d.S3.Prefix),
			AuthType: string(d.S3.AuthType), RoleArn: strDerefPtr(d.S3.RoleArn), ExternalID: strDerefPtr(d.S3.ExternalId),
			HasAccessKeySecret: d.S3.HasAccessKeySecret,
		}
	}
	if d.Http != nil {
		out.HTTP = &SIEMHTTP{
			URL: d.Http.Url, AuthType: string(d.Http.AuthType),
			HasAuthCredentials: d.Http.HasAuthCredentials, HeaderNames: d.Http.HeaderNames,
		}
	}
	return out
}

// ListSIEMDestinations returns the workspace's SIEM destinations.
func (c *Client) ListSIEMDestinations(ctx context.Context) ([]SIEMDestination, error) {
	resp, err := c.gen.ListSIEMDestinationsWithResponse(ctx, &apiclient.ListSIEMDestinationsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]SIEMDestination, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, *siemFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetSIEMDestination returns a single destination by ID.
func (c *Client) GetSIEMDestination(ctx context.Context, id string) (*SIEMDestination, error) {
	resp, err := c.gen.GetSIEMDestinationWithResponse(ctx, did(id), &apiclient.GetSIEMDestinationParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return siemFromAPI(resp.JSON200), nil
}

// CreateSIEMDestination creates a SIEM forwarding destination.
func (c *Client) CreateSIEMDestination(ctx context.Context, req CreateSIEMDestinationRequest) (*SIEMDestination, error) {
	body := apiclient.CreateSIEMDestinationJSONRequestBody{
		Name: req.Name, Enabled: req.Enabled, Type: apiclient.SIEMDestinationType(req.Type),
		Source: siemSourceToAPI(req.Source), Batch: siemBatchToAPI(req.Batch),
		Syslog: siemSyslogToAPI(req.Syslog), S3: siemS3ToAPI(req.S3), Http: siemHTTPToAPI(req.HTTP),
	}
	resp, err := c.gen.CreateSIEMDestinationWithResponse(ctx, &apiclient.CreateSIEMDestinationParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return siemFromAPI(resp.JSON201), nil
}

// UpdateSIEMDestination updates a destination in place. All definitional
// fields are sent so the destination converges on the desired state.
func (c *Client) UpdateSIEMDestination(ctx context.Context, id string, req CreateSIEMDestinationRequest) (*SIEMDestination, error) {
	name := req.Name
	src := siemSourceToAPI(req.Source)
	body := apiclient.UpdateSIEMDestinationJSONRequestBody{
		Name: &name, Enabled: req.Enabled, Source: &src, Batch: siemBatchToAPI(req.Batch),
		Syslog: siemSyslogToAPI(req.Syslog), S3: siemS3ToAPI(req.S3), Http: siemHTTPToAPI(req.HTTP),
	}
	resp, err := c.gen.UpdateSIEMDestinationWithResponse(ctx, did(id), &apiclient.UpdateSIEMDestinationParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return siemFromAPI(resp.JSON200), nil
}

// DeleteSIEMDestination deletes a destination.
func (c *Client) DeleteSIEMDestination(ctx context.Context, id string) error {
	resp, err := c.gen.DeleteSIEMDestinationWithResponse(ctx, did(id), &apiclient.DeleteSIEMDestinationParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// TestSIEMDestination sends a synthetic event through a saved destination.
func (c *Client) TestSIEMDestination(ctx context.Context, id string) (*SIEMTestResult, error) {
	resp, err := c.gen.TestSIEMDestinationWithResponse(ctx, did(id), &apiclient.TestSIEMDestinationParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return &SIEMTestResult{Success: resp.JSON200.Success, Message: strDerefPtr(resp.JSON200.Message)}, nil
}
