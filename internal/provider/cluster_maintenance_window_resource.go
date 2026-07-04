package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*clusterMaintenanceWindowResource)(nil)
	_ resource.ResourceWithConfigure   = (*clusterMaintenanceWindowResource)(nil)
	_ resource.ResourceWithImportState = (*clusterMaintenanceWindowResource)(nil)
)

type clusterMaintenanceWindowResource struct{ client *skycloak.Client }

// NewClusterMaintenanceWindowResource returns the skycloak_cluster_maintenance_window resource.
func NewClusterMaintenanceWindowResource() resource.Resource {
	return &clusterMaintenanceWindowResource{}
}

type maintenanceWindowModel struct {
	ID         types.String `tfsdk:"id"`
	ClusterID  types.String `tfsdk:"cluster_id"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	DaysOfWeek []types.Int64 `tfsdk:"days_of_week"`
	StartLocal types.String `tfsdk:"start_local"`
	EndLocal   types.String `tfsdk:"end_local"`
	Timezone   types.String `tfsdk:"timezone"`
}

func (r *clusterMaintenanceWindowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_maintenance_window"
}

func (r *clusterMaintenanceWindowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The cluster's own maintenance window: when Skycloak applies upgrades and other disruptive changes. " +
			"A singleton per cluster (create == update upsert). Destroying this resource does not leave the cluster without " +
			"a window: it reverts the cluster to the workspace default window.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/maintenance-window`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID. Immutable.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{Required: true, MarkdownDescription: "Whether the window is active."},
			"days_of_week": schema.ListAttribute{
				Required:            true,
				ElementType:         types.Int64Type,
				MarkdownDescription: "Days of week the window applies to (0 = Sunday ... 6 = Saturday).",
			},
			"start_local": schema.StringAttribute{Required: true, MarkdownDescription: "Local start time, `HH:MM`."},
			"end_local":   schema.StringAttribute{Required: true, MarkdownDescription: "Local end time, `HH:MM`."},
			"timezone":    schema.StringAttribute{Required: true, MarkdownDescription: "IANA timezone, e.g. `Europe/Berlin`."},
		},
	}
}

func (r *clusterMaintenanceWindowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*skycloak.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *skycloak.Client, got %T", req.ProviderData))
		return
	}
	r.client = client
}

func (m *maintenanceWindowModel) toRequest() skycloak.MaintenanceWindow {
	days := make([]int64, 0, len(m.DaysOfWeek))
	for _, d := range m.DaysOfWeek {
		days = append(days, d.ValueInt64())
	}
	return skycloak.MaintenanceWindow{
		Enabled: m.Enabled.ValueBool(), DaysOfWeek: days,
		StartLocal: m.StartLocal.ValueString(), EndLocal: m.EndLocal.ValueString(),
		Timezone: m.Timezone.ValueString(),
	}
}

func applyWindowToModel(w *skycloak.MaintenanceWindow, m *maintenanceWindowModel) {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/maintenance-window")
	m.Enabled = types.BoolValue(w.Enabled)
	m.DaysOfWeek = make([]types.Int64, 0, len(w.DaysOfWeek))
	for _, d := range w.DaysOfWeek {
		m.DaysOfWeek = append(m.DaysOfWeek, types.Int64Value(d))
	}
	m.StartLocal = types.StringValue(w.StartLocal)
	m.EndLocal = types.StringValue(w.EndLocal)
	m.Timezone = types.StringValue(w.Timezone)
}

func (r *clusterMaintenanceWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.SetMaintenanceWindow(ctx, plan.ClusterID.ValueString(), plan.toRequest())
	if err != nil {
		resp.Diagnostics.AddError("Unable to set cluster maintenance window", err.Error())
		return
	}
	applyWindowToModel(w, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterMaintenanceWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.GetMaintenanceWindow(ctx, state.ClusterID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read cluster maintenance window", err.Error())
		return
	}
	applyWindowToModel(w, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterMaintenanceWindowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan maintenanceWindowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.SetMaintenanceWindow(ctx, plan.ClusterID.ValueString(), plan.toRequest())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update cluster maintenance window", err.Error())
		return
	}
	applyWindowToModel(w, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterMaintenanceWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state maintenanceWindowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMaintenanceWindow(ctx, state.ClusterID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete cluster maintenance window", err.Error())
	}
}

func (r *clusterMaintenanceWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by cluster ID (the window is a singleton under the cluster).
	clusterID := strings.TrimSuffix(req.ID, "/maintenance-window")
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), clusterID+"/maintenance-window")...)
}
