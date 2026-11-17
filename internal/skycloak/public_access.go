package skycloak

import (
	"context"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// GetClusterPublicAccess reports whether a cluster's public hostname is served.
// A cluster that does not report the setting is served publicly.
func (c *Client) GetClusterPublicAccess(ctx context.Context, clusterID string) (bool, error) {
	resp, err := c.gen.GetClusterWithResponse(ctx, cid(clusterID), &apiclient.GetClusterParams{APIVersion: c.ver()})
	if err != nil {
		return false, err
	}
	if resp.JSON200 == nil {
		return false, statusError(resp.HTTPResponse, resp.Body)
	}
	if resp.JSON200.PublicAccess == nil {
		return true, nil
	}
	return *resp.JSON200.PublicAccess, nil
}

// SetClusterPublicAccess turns a cluster's public hostname on or off and returns the resulting setting.
func (c *Client) SetClusterPublicAccess(ctx context.Context, clusterID string, enabled bool) (bool, error) {
	body := apiclient.UpdateClusterJSONRequestBody{PublicAccess: &enabled}
	resp, err := c.gen.UpdateClusterWithResponse(ctx, cid(clusterID), &apiclient.UpdateClusterParams{APIVersion: c.ver()}, body)
	if err != nil {
		return false, err
	}
	if resp.JSON200 == nil {
		return false, statusError(resp.HTTPResponse, resp.Body)
	}
	if resp.JSON200.PublicAccess == nil {
		return true, nil
	}
	return *resp.JSON200.PublicAccess, nil
}
