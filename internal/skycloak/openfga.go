package skycloak

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// OpenFGA instance lifecycle states, as the API reports them.
const (
	OpenFGAStatusProvisioning = "provisioning"
	OpenFGAStatusReady        = "ready"
	OpenFGAStatusFailed       = "failed"
	OpenFGAStatusDeleting     = "deleting"
)

// openFGAPollInterval is how often the wait helpers re-read an instance. It is
// a variable so tests can shorten it.
var openFGAPollInterval = 10 * time.Second

// OpenFGAInstance mirrors the public API OpenFGAInstance resource.
type OpenFGAInstance struct {
	ID            string
	ClusterID     string
	Realm         string
	Status        string
	StatusMessage string
	APIURL        string
	Issuer        string
	Audience      string
	CreatedAt     string
	UpdatedAt     string
}

func openFGAInstanceFromAPI(i *apiclient.OpenFGAInstance) *OpenFGAInstance {
	out := &OpenFGAInstance{
		ID:        i.Id.String(),
		ClusterID: i.ClusterId.String(),
		Realm:     i.Realm,
		Status:    string(i.Status),
		APIURL:    i.ApiUrl,
		Issuer:    i.Issuer,
		Audience:  i.Audience,
		CreatedAt: fmtTime(i.CreatedAt),
		UpdatedAt: fmtTime(i.UpdatedAt),
	}
	if i.StatusMessage != nil {
		out.StatusMessage = *i.StatusMessage
	}
	return out
}

func instanceUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid OpenFGA instance ID %q: %w", s, err)
	}
	return id, nil
}

// ListOpenFGAInstances returns the OpenFGA instances bound to the cluster's realms.
func (c *Client) ListOpenFGAInstances(ctx context.Context, clusterID string) ([]OpenFGAInstance, error) {
	resp, err := c.gen.ListOpenFGAInstancesWithResponse(ctx, cid(clusterID), &apiclient.ListOpenFGAInstancesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]OpenFGAInstance, 0, len(resp.JSON200.Instances))
	for i := range resp.JSON200.Instances {
		out = append(out, *openFGAInstanceFromAPI(&resp.JSON200.Instances[i]))
	}
	return out, nil
}

// GetOpenFGAInstance returns one OpenFGA instance.
func (c *Client) GetOpenFGAInstance(ctx context.Context, clusterID, instanceID string) (*OpenFGAInstance, error) {
	id, err := instanceUUID(instanceID)
	if err != nil {
		return nil, err
	}
	resp, err := c.gen.GetOpenFGAInstanceWithResponse(ctx, cid(clusterID), id, &apiclient.GetOpenFGAInstanceParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return openFGAInstanceFromAPI(resp.JSON200), nil
}

// CreateOpenFGAInstance binds a new OpenFGA instance to a realm. The API answers
// 202 while the instance provisions; use WaitForOpenFGAInstanceReady to follow it.
func (c *Client) CreateOpenFGAInstance(ctx context.Context, clusterID, realm string) (*OpenFGAInstance, error) {
	resp, err := c.gen.CreateOpenFGAInstanceWithResponse(ctx, cid(clusterID), &apiclient.CreateOpenFGAInstanceParams{APIVersion: c.ver()},
		apiclient.CreateOpenFGAInstanceRequest{Realm: realm})
	if err != nil {
		return nil, err
	}
	if resp.JSON202 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return openFGAInstanceFromAPI(resp.JSON202), nil
}

// DeleteOpenFGAInstance removes an instance and its data. The API answers 202
// while it is removed and 204 when it is already gone.
func (c *Client) DeleteOpenFGAInstance(ctx context.Context, clusterID, instanceID string) error {
	id, err := instanceUUID(instanceID)
	if err != nil {
		return err
	}
	resp, err := c.gen.DeleteOpenFGAInstanceWithResponse(ctx, cid(clusterID), id, &apiclient.DeleteOpenFGAInstanceParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// WaitForOpenFGAInstanceReady polls the instance until it is "ready", returns
// the instance with an error if it reaches "failed", or surfaces the context error.
func (c *Client) WaitForOpenFGAInstanceReady(ctx context.Context, clusterID, instanceID string) (*OpenFGAInstance, error) {
	ticker := time.NewTicker(openFGAPollInterval)
	defer ticker.Stop()
	for {
		inst, err := c.GetOpenFGAInstance(ctx, clusterID, instanceID)
		if err != nil {
			return nil, err
		}
		switch inst.Status {
		case OpenFGAStatusReady:
			return inst, nil
		case OpenFGAStatusFailed:
			msg := ""
			if inst.StatusMessage != "" {
				msg = ": " + inst.StatusMessage
			}
			return inst, fmt.Errorf("OpenFGA instance %s entered failed state%s", instanceID, msg)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// WaitForOpenFGAInstanceGone polls until the instance no longer exists.
func (c *Client) WaitForOpenFGAInstanceGone(ctx context.Context, clusterID, instanceID string) error {
	ticker := time.NewTicker(openFGAPollInterval)
	defer ticker.Stop()
	for {
		_, err := c.GetOpenFGAInstance(ctx, clusterID, instanceID)
		if err != nil {
			if IsNotFound(err) {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
