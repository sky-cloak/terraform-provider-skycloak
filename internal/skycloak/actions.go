package skycloak

import (
	"context"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// ActionResult is the outcome of an imperative verification action.
type ActionResult struct {
	Success bool
	Message string
	Details map[string]string
}

// TestSMTP sends a probe email through a realm's saved SMTP configuration.
func (c *Client) TestSMTP(ctx context.Context, clusterID, realm, email string) (*ActionResult, error) {
	resp, err := c.gen.TestSmtpConfigWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm),
		&apiclient.TestSmtpConfigParams{APIVersion: c.ver()},
		apiclient.SmtpTestRequest{Email: openapitypes.Email(email)})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return &ActionResult{Success: resp.JSON200.Success, Message: strDerefPtr(resp.JSON200.Message)}, nil
}

// TestIdentityProvider validates an identity provider's connection, optionally
// overriding the client credentials for this test only.
func (c *Client) TestIdentityProvider(ctx context.Context, clusterID, realm, providerID, clientID, clientSecret string) (*ActionResult, error) {
	body := apiclient.TestProviderConnectionRequest{}
	body.ClientId = strPtr(clientID)
	body.ClientSecret = strPtr(clientSecret)
	resp, err := c.gen.TestIdentityProviderConnectionWithResponse(ctx, cid(clusterID), apiclient.RealmName(realm), providerID,
		&apiclient.TestIdentityProviderConnectionParams{APIVersion: c.ver()}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := &ActionResult{Success: resp.JSON200.Success, Message: resp.JSON200.Message}
	if resp.JSON200.Details != nil {
		out.Details = *resp.JSON200.Details
	}
	return out, nil
}

// CancelClusterUpgrade cancels a scheduled or in-progress cluster upgrade and
// returns the cluster's state after the cancellation was accepted.
func (c *Client) CancelClusterUpgrade(ctx context.Context, clusterID string) (*Cluster, error) {
	resp, err := c.gen.CancelClusterUpgradeWithResponse(ctx, cid(clusterID), &apiclient.CancelClusterUpgradeParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return clusterFromAPI(&resp.JSON200.Cluster), nil
}
