package skycloak

import (
	"context"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// CAPTCHADomain is a hostname registered for CAPTCHA protection.
type CAPTCHADomain struct {
	Hostname  string
	CreatedAt string
}

// CAPTCHADomains lists a cluster's registered CAPTCHA hostnames and the
// registration ceiling.
type CAPTCHADomains struct {
	Domains    []CAPTCHADomain
	MaxAllowed int64
}

// ListCAPTCHADomains returns the hostnames registered for CAPTCHA protection.
func (c *Client) ListCAPTCHADomains(ctx context.Context, clusterID string) (*CAPTCHADomains, error) {
	resp, err := c.gen.ListClusterCAPTCHADomainsWithResponse(ctx, cid(clusterID), &apiclient.ListClusterCAPTCHADomainsParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := &CAPTCHADomains{MaxAllowed: int64(resp.JSON200.MaxAllowed)}
	for _, d := range resp.JSON200.Domains {
		out.Domains = append(out.Domains, CAPTCHADomain{Hostname: d.Hostname, CreatedAt: fmtTime(d.CreatedAt)})
	}
	return out, nil
}

// AddCAPTCHADomain registers a hostname for CAPTCHA protection.
func (c *Client) AddCAPTCHADomain(ctx context.Context, clusterID, hostname string) (*CAPTCHADomain, error) {
	resp, err := c.gen.AddClusterCAPTCHADomainWithResponse(ctx, cid(clusterID), &apiclient.AddClusterCAPTCHADomainParams{APIVersion: c.ver()},
		apiclient.AddCAPTCHADomainRequest{Hostname: hostname})
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return &CAPTCHADomain{Hostname: resp.JSON201.Hostname, CreatedAt: fmtTime(resp.JSON201.CreatedAt)}, nil
}

// RemoveCAPTCHADomain unregisters a hostname.
func (c *Client) RemoveCAPTCHADomain(ctx context.Context, clusterID, hostname string) error {
	resp, err := c.gen.RemoveClusterCAPTCHADomainWithResponse(ctx, cid(clusterID), hostname, &apiclient.RemoveClusterCAPTCHADomainParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}
