package skycloak

import (
	"context"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// MaintenanceWindow is when Skycloak applies upgrades and other disruptive
// changes to a cluster.
type MaintenanceWindow struct {
	Enabled    bool
	DaysOfWeek []int64
	StartLocal string
	EndLocal   string
	Timezone   string
}

func maintenanceWindowFromAPI(w *apiclient.MaintenanceWindow) *MaintenanceWindow {
	days := make([]int64, 0, len(w.DaysOfWeek))
	for _, d := range w.DaysOfWeek {
		days = append(days, int64(d))
	}
	return &MaintenanceWindow{
		Enabled: w.Enabled, DaysOfWeek: days,
		StartLocal: w.StartLocal, EndLocal: w.EndLocal, Timezone: w.Timezone,
	}
}

func (w *MaintenanceWindow) toAPI() apiclient.MaintenanceWindow {
	days := make([]int32, 0, len(w.DaysOfWeek))
	for _, d := range w.DaysOfWeek {
		days = append(days, int32(d))
	}
	return apiclient.MaintenanceWindow{
		Enabled: w.Enabled, DaysOfWeek: days,
		StartLocal: w.StartLocal, EndLocal: w.EndLocal, Timezone: w.Timezone,
	}
}

// GetMaintenanceWindow returns a cluster's own maintenance window.
func (c *Client) GetMaintenanceWindow(ctx context.Context, clusterID string) (*MaintenanceWindow, error) {
	resp, err := c.gen.GetClusterMaintenanceWindowWithResponse(ctx, cid(clusterID), &apiclient.GetClusterMaintenanceWindowParams{APIVersion: c.ver()})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return maintenanceWindowFromAPI(resp.JSON200), nil
}

// SetMaintenanceWindow upserts a cluster's maintenance window.
func (c *Client) SetMaintenanceWindow(ctx context.Context, clusterID string, w MaintenanceWindow) (*MaintenanceWindow, error) {
	resp, err := c.gen.SetClusterMaintenanceWindowWithResponse(ctx, cid(clusterID), &apiclient.SetClusterMaintenanceWindowParams{APIVersion: c.ver()}, w.toAPI())
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return maintenanceWindowFromAPI(resp.JSON200), nil
}

// DeleteMaintenanceWindow removes a cluster's own window; the cluster then
// follows the workspace default.
func (c *Client) DeleteMaintenanceWindow(ctx context.Context, clusterID string) error {
	resp, err := c.gen.DeleteClusterMaintenanceWindowWithResponse(ctx, cid(clusterID), &apiclient.DeleteClusterMaintenanceWindowParams{APIVersion: c.ver()})
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}
