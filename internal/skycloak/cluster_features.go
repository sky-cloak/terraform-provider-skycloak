package skycloak

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// Cluster feature rollout states reported by the API.
const (
	ClusterFeatureChangePending  = "pending"
	ClusterFeatureChangeApplying = "applying"
)

// ClusterFeatures is the requested set of customer-managed enabled flags.
type ClusterFeatures struct {
	Features []string
	// Change is empty when the API reports no rollout in progress.
	Change string
}

func clusterFeaturesFromAPI(features []string, change *apiclient.ClusterFeatureChange) *ClusterFeatures {
	result := &ClusterFeatures{Features: append([]string{}, features...)}
	if change != nil {
		result.Change = string(change.State)
	}
	return result
}

// GetClusterFeatures reads the requested flags and rollout state.
func (c *Client) GetClusterFeatures(ctx context.Context, clusterID string) (*ClusterFeatures, error) {
	resp, err := c.gen.GetClusterFeaturesWithResponse(ctx, cid(clusterID), &apiclient.GetClusterFeaturesParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return clusterFeaturesFromAPI(resp.JSON200.Features, resp.JSON200.Change), nil
}

// ReplaceClusterFeatures replaces the full set of customer-managed enabled flags.
func (c *Client) ReplaceClusterFeatures(ctx context.Context, clusterID string, features []string, applyNow bool) (*ClusterFeatures, error) {
	if features == nil {
		features = []string{}
	}
	resp, err := c.gen.ReplaceClusterFeaturesWithResponse(ctx, cid(clusterID),
		&apiclient.ReplaceClusterFeaturesParams{APIVersion: c.ver()},
		apiclient.ReplaceClusterFeaturesRequest{Features: features, ApplyNow: &applyNow})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return clusterFeaturesFromAPI(resp.JSON200.Features, resp.JSON200.Change), nil
}

// WaitForClusterFeatures waits for the requested flags to finish rolling out.
func (c *Client) WaitForClusterFeatures(ctx context.Context, clusterID string, features []string) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	want := slices.Clone(features)
	slices.Sort(want)

	for {
		current, err := c.GetClusterFeatures(ctx, clusterID)
		if err != nil {
			return err
		}
		slices.Sort(current.Features)
		if !slices.Equal(current.Features, want) {
			return fmt.Errorf("cluster %s feature flags changed during the rollout", clusterID)
		}
		if current.Change == "" {
			// A failed cluster also has no rollout status.
			cluster, err := c.GetCluster(ctx, clusterID)
			if err != nil {
				return err
			}
			switch cluster.Status {
			case "available":
				return nil
			case "failed", "stopped", "deleting":
				return fmt.Errorf("cluster %s entered %s state during the feature rollout", clusterID, cluster.Status)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
