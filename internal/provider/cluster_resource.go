package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// createTimeout bounds the wait for a new cluster to become available.
const createTimeout = 30 * time.Minute

var (
	_ resource.Resource                = (*clusterResource)(nil)
	_ resource.ResourceWithConfigure   = (*clusterResource)(nil)
	_ resource.ResourceWithImportState = (*clusterResource)(nil)
)

type clusterResource struct {
	client *skycloak.Client
}

// NewClusterResource returns the skycloak_cluster resource.
func NewClusterResource() resource.Resource {
	return &clusterResource{}
}

type clusterModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Type               types.String `tfsdk:"type"`
	Size               types.String `tfsdk:"size"`
	Version            types.String `tfsdk:"version"`
	Location           types.String `tfsdk:"location"`
	Status             types.String `tfsdk:"status"`
	URL                types.String `tfsdk:"url"`
	AutoUpgradeEnabled types.Bool   `tfsdk:"auto_upgrade_enabled"`
}

func (r *clusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *clusterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Skycloak managed Keycloak cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster ID (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Human-readable cluster name.",
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster type (`keycloak` or `tidecloak`). Immutable — changing it replaces the cluster.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"size": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Instance size (`small`, `medium`, `large`).",
			},
			"version": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Keycloak version, e.g. `26.1`.",
			},
			"location": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Region (`us` for US East, `us-west` for US West, `ca`, `eu`, `au`). Immutable: changing it replaces the cluster. " +
					"The one exception is a cluster the API reports as `us-west`: switching its configuration between `us` and `us-west` is applied in place, because both name the same cluster.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIf(
					requiresReplaceUnlessUSWestRelabel,
					"Changing the location replaces the cluster, except relabelling a US West cluster between us and us-west.",
					"Changing the location replaces the cluster, except relabelling a US West cluster between `us` and `us-west`.",
				)},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Lifecycle status (`provisioning`, `available`, ...).",
			},
			"url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Cluster base URL once available.",
			},
			"auto_upgrade_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Enable automatic patch upgrades, applied inside the cluster's maintenance window. Defaults to `false`.",
			},
		},
	}
}

func (r *clusterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *clusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	autoUpgrade := plan.AutoUpgradeEnabled.ValueBool()
	created, err := r.client.CreateCluster(ctx, skycloak.CreateClusterRequest{
		Name:               plan.Name.ValueString(),
		Type:               plan.Type.ValueString(),
		Size:               plan.Size.ValueString(),
		Version:            plan.Version.ValueString(),
		Location:           plan.Location.ValueString(),
		AutoUpgradeEnabled: &autoUpgrade,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create cluster", err.Error())
		return
	}

	// Creation is async (202). Poll until available, bounded by createTimeout.
	waitCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	final, err := r.client.WaitForClusterReady(waitCtx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Cluster did not become available", err.Error())
		// Persist the ID so the half-created cluster is tracked, not leaked.
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), created.ID)...)
		return
	}

	planned := plan.Location.ValueString()
	applyClusterToModel(final, &plan)
	plan.Location = types.StringValue(reconcileClusterLocation(planned, final.Location, &resp.Diagnostics))
	resp.Diagnostics.Append(setAPILocation(ctx, resp.Private, final.Location)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cl, err := r.client.GetCluster(ctx, state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read cluster", err.Error())
		return
	}

	prior := state.Location.ValueString()
	applyClusterToModel(cl, &state)
	state.Location = types.StringValue(reconcileClusterLocation(prior, cl.Location, &resp.Diagnostics))
	resp.Diagnostics.Append(setAPILocation(ctx, resp.Private, cl.Location)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state clusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updated *skycloak.Cluster
	var err error
	if plan.Size.Equal(state.Size) && plan.Version.Equal(state.Version) && plan.AutoUpgradeEnabled.Equal(state.AutoUpgradeEnabled) {
		// Nothing the API can update changed: in practice this is the
		// us <-> us-west relabel of a US West cluster, the one location change
		// that is not a replacement. Read the cluster instead of patching it.
		updated, err = r.client.GetCluster(ctx, plan.ID.ValueString())
	} else {
		size := plan.Size.ValueString()
		version := plan.Version.ValueString()
		autoUpgrade := plan.AutoUpgradeEnabled.ValueBool()
		updated, err = r.client.UpdateCluster(ctx, plan.ID.ValueString(), skycloak.UpdateClusterRequest{
			Size:               &size,
			Version:            &version,
			AutoUpgradeEnabled: &autoUpgrade,
		})
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to update cluster", err.Error())
		return
	}

	planned := plan.Location.ValueString()
	applyClusterToModel(updated, &plan)
	plan.Location = types.StringValue(reconcileClusterLocation(planned, updated.Location, &resp.Diagnostics))
	resp.Diagnostics.Append(setAPILocation(ctx, resp.Private, updated.Location)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteCluster(ctx, state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete cluster", err.Error())
	}
}

func (r *clusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func applyClusterToModel(c *skycloak.Cluster, m *clusterModel) {
	m.ID = types.StringValue(c.ID)
	m.Name = types.StringValue(c.Name)
	m.Type = types.StringValue(c.Type)
	m.Size = types.StringValue(c.Size)
	m.Version = types.StringValue(c.Version)
	m.Location = types.StringValue(c.Location)
	m.Status = types.StringValue(c.Status)
	m.URL = types.StringValue(c.URL)
	m.AutoUpgradeEnabled = types.BoolValue(c.AutoUpgradeEnabled)
}
