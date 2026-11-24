package skycloak

import (
	"context"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// ClusterPrivateLink is a cluster's Azure private link configuration.
type ClusterPrivateLink struct {
	Enabled              bool
	AzureSubscriptionIDs []string
	Hostnames            []string
	Alias                *string
}

func privateLinkFromAPI(link *apiclient.ClusterPrivateLink) *ClusterPrivateLink {
	out := &ClusterPrivateLink{Enabled: link.Enabled, AzureSubscriptionIDs: link.AzureSubscriptionIds, Alias: link.Alias}
	if link.Hostnames != nil {
		out.Hostnames = *link.Hostnames
	}
	return out
}

// GetClusterPrivateLink returns a cluster's private link configuration.
func (c *Client) GetClusterPrivateLink(ctx context.Context, clusterID string) (*ClusterPrivateLink, error) {
	resp, err := c.gen.GetClusterPrivateLinkWithResponse(ctx, cid(clusterID), &apiclient.GetClusterPrivateLinkParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return privateLinkFromAPI(resp.JSON200), nil
}

// UpdateClusterPrivateLink sets enabled state and, when non-nil, replaces the subscription IDs and the hostnames.
// An empty non-nil hostnames slice clears the stored hostnames.
func (c *Client) UpdateClusterPrivateLink(ctx context.Context, clusterID string, enabled bool, subscriptionIDs, hostnames []string) (*ClusterPrivateLink, error) {
	body := apiclient.UpdateClusterPrivateLinkJSONRequestBody{Enabled: enabled}
	if subscriptionIDs != nil {
		body.AzureSubscriptionIds = &subscriptionIDs
	}
	if hostnames != nil {
		body.Hostnames = &hostnames
	}
	resp, err := c.gen.UpdateClusterPrivateLinkWithResponse(ctx, cid(clusterID), &apiclient.UpdateClusterPrivateLinkParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return privateLinkFromAPI(resp.JSON200), nil
}
