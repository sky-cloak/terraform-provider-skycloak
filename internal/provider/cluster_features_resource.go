package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

const clusterFeaturesRestartTimeout = 30 * time.Minute

var (
	_ resource.Resource                = (*clusterFeaturesResource)(nil)
	_ resource.ResourceWithConfigure   = (*clusterFeaturesResource)(nil)
	_ resource.ResourceWithImportState = (*clusterFeaturesResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*clusterFeaturesResource)(nil)
)

type clusterFeaturesResource struct{ client *skycloak.Client }

// NewClusterFeaturesResource returns the skycloak_cluster_features resource.
func NewClusterFeaturesResource() resource.Resource { return &clusterFeaturesResource{} }

type clusterFeaturesResourceModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	Features  types.Set    `tfsdk:"features"`
	ApplyNow  types.Bool   `tfsdk:"apply_now"`
}

func (r *clusterFeaturesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_features"
}

func (r *clusterFeaturesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the full set of customer-managed enabled Keycloak feature flags on a cluster. " +
			"Use one resource per cluster. Removing a flag turns it off. Destroy clears all customer-managed enabled flags. " +
			"Skycloak-managed flags are preserved. Changes restart Keycloak. " +
			"The API key needs the `clusters:read` and `clusters:write` scopes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/features`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID. Immutable.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"features": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Complete set of enabled flags. Use an empty set to turn them all off. See the `skycloak_cluster_features` data source for the catalog and version requirements.",
			},
			"apply_now": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Set to `true` to restart immediately, including for changes already pending. Defaults to `false`, which schedules changes for the " +
					"maintenance window when one is enabled. Without an enabled window, changes apply immediately. Terraform waits for immediate restarts to finish. " +
					"This setting also controls the restart on destroy and defaults to `false` on import.",
			},
		},
	}
}

func (r *clusterFeaturesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *clusterFeaturesResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	var state, plan clusterFeaturesResourceModel
	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	}
	if !req.Plan.Raw.IsNull() {
		resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if req.Plan.Raw.IsNull() {
		if len(state.Features.Elements()) == 0 {
			return
		}
	} else if !req.State.Raw.IsNull() && plan.ClusterID.Equal(state.ClusterID) &&
		plan.Features.Equal(state.Features) && (!plan.ApplyNow.ValueBool() || state.ApplyNow.ValueBool()) {
		return
	}

	resp.Diagnostics.AddWarning("Cluster feature changes can restart Keycloak",
		"Keycloak restarts if the enabled feature flags change or a pending change is applied. With apply_now enabled, the restart can occur outside the maintenance window.")
}

func (r *clusterFeaturesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterFeaturesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.set(ctx, &plan, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *clusterFeaturesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterFeaturesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	flags, err := r.client.GetClusterFeatures(ctx, state.ClusterID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read cluster feature flags", err.Error())
		return
	}
	flagsSet, diags := types.SetValueFrom(ctx, types.StringType, flags.Features)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Features = flagsSet
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterFeaturesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state clusterFeaturesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.Features.Equal(state.Features) || plan.ApplyNow.ValueBool() && !state.ApplyNow.ValueBool() {
		r.set(ctx, &plan, &resp.Diagnostics)
	}
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *clusterFeaturesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterFeaturesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.replace(ctx, state.ClusterID.ValueString(), nil, state.ApplyNow.ValueBool())
	if err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to clear cluster feature flags", err.Error())
	}
}

func (r *clusterFeaturesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID := strings.TrimSuffix(req.ID, "/features")
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), clusterID+"/features")...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("apply_now"), false)...)
}

func (r *clusterFeaturesResource) set(ctx context.Context, model *clusterFeaturesResourceModel, diagnostics *diag.Diagnostics) {
	features := make([]string, 0, len(model.Features.Elements()))
	diagnostics.Append(model.Features.ElementsAs(ctx, &features, false)...)
	if diagnostics.HasError() {
		return
	}

	clusterID := model.ClusterID.ValueString()
	if err := r.replace(ctx, clusterID, features, model.ApplyNow.ValueBool()); err != nil {
		diagnostics.AddError("Unable to set cluster feature flags", err.Error())
		return
	}
	model.ID = types.StringValue(clusterID + "/features")
}

func (r *clusterFeaturesResource) replace(ctx context.Context, clusterID string, features []string, applyNow bool) error {
	slices.Sort(features)
	result, err := r.client.ReplaceClusterFeatures(ctx, clusterID, features, applyNow)
	if err != nil {
		return err
	}
	slices.Sort(result.Features)
	if !slices.Equal(result.Features, features) {
		return fmt.Errorf("the API returned a different set of enabled flags than requested")
	}
	switch result.Change {
	case skycloak.ClusterFeatureChangePending:
		if applyNow {
			return fmt.Errorf("the API scheduled the restart for the maintenance window instead of starting it immediately")
		}
	case skycloak.ClusterFeatureChangeApplying:
		waitCtx, cancel := context.WithTimeout(ctx, clusterFeaturesRestartTimeout)
		defer cancel()
		if err := r.client.WaitForClusterFeatures(waitCtx, clusterID, features); err != nil {
			return fmt.Errorf("the API saved the requested feature flags. Waiting for Keycloak restart failed: %w", err)
		}
	}
	return nil
}
