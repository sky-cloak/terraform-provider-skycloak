package skycloak

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/platformclient"
)

// DefaultPlatformEndpoint is the server address in the Platform API spec.
const DefaultPlatformEndpoint = "https://api.skycloak.io"

// PlatformClient wraps the generated Platform API client. It carries the same
// retry transport, timeout and User-Agent as Client and authenticates with a
// Platform API key sent as a bearer token.
type PlatformClient struct {
	gen *platformclient.ClientWithResponses
}

// NewPlatform builds a PlatformClient. endpoint defaults to
// DefaultPlatformEndpoint when empty.
func NewPlatform(endpoint, apiKey string, opts ...Option) *PlatformClient {
	cfg := &config{httpClient: &http.Client{Timeout: 30 * time.Second}, userAgent: "skycloak-go/dev"}
	for _, o := range opts {
		o(cfg)
	}
	if endpoint == "" {
		endpoint = DefaultPlatformEndpoint
	}
	cfg.httpClient.Transport = &retryTransport{base: cfg.httpClient.Transport, maxRetries: 4}

	editor := func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")
		if cfg.userAgent != "" {
			req.Header.Set("User-Agent", cfg.userAgent)
		}
		return nil
	}

	gen, err := platformclient.NewClientWithResponses(endpoint,
		platformclient.WithHTTPClient(cfg.httpClient),
		platformclient.WithRequestEditorFn(editor),
	)
	if err != nil {
		// NewClientWithResponses only errors on an unparseable server URL.
		panic(fmt.Sprintf("skycloak: invalid platform endpoint %q: %v", endpoint, err))
	}
	return &PlatformClient{gen: gen}
}

// API returns the generated Platform API client for the calls the facade does
// not wrap.
func (c *PlatformClient) API() *platformclient.ClientWithResponses { return c.gen }
