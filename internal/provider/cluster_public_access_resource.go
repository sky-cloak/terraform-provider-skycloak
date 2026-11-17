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
	_ resource.Resource                = (*clusterPublicAccessResource)(nil)
	_ resource.ResourceWithConfigure   = (*clusterPublicAccessResource)(nil)
	_ resource.ResourceWithImportState = (*clusterPublicAccessResource)(nil)
)

type clusterPublicAccessResource struct{ client *skycloak.Client }

// NewClusterPublicAccessResource returns the skycloak_cluster_public_access resource.
func NewClusterPublicAccessResource() resource.Resource { return &clusterPublicAccessResource{} }

type clusterPublicAccessModel struct {
	ID           types.String `tfsdk:"id"`
	ClusterID    types.String `tfsdk:"cluster_id"`
	PublicAccess types.Bool   `tfsdk:"public_access"`
}

func (r *clusterPublicAccessResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_public_access"
}

func (r *clusterPublicAccessResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages whether a cluster's public hostname is served. Only one resource should manage a cluster's public access. " +
			"Turning public access off needs an Enterprise plan with private networking, and the API refuses it unless Azure private link is enabled and ready, " +
			"because the private hostname is then the only way in. Order this resource after `skycloak_cluster_private_link`. " +
			"Destroy turns public access back on, which lets the private link be disabled afterwards. " +
			"The API key needs the `clusters:read` and `clusters:write` scopes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/public-access`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID. Immutable.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_access": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether the cluster's public hostname is served. When `false` the cluster is reachable only through its private link hostname.",
			},
		},
	}
}

func (r *clusterPublicAccessResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *clusterPublicAccessResource) set(ctx context.Context, m *clusterPublicAccessModel) error {
	enabled, err := r.client.SetClusterPublicAccess(ctx, m.ClusterID.ValueString(), m.PublicAccess.ValueBool())
	if err != nil {
		return err
	}
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/public-access")
	m.PublicAccess = types.BoolValue(enabled)
	return nil
}

func (r *clusterPublicAccessResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterPublicAccessModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.set(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to set cluster public access", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterPublicAccessResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterPublicAccessModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	enabled, err := r.client.GetClusterPublicAccess(ctx, state.ClusterID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read cluster public access", err.Error())
		return
	}
	state.ID = types.StringValue(state.ClusterID.ValueString() + "/public-access")
	state.PublicAccess = types.BoolValue(enabled)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterPublicAccessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterPublicAccessModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.set(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to set cluster public access", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterPublicAccessResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterPublicAccessModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.SetClusterPublicAccess(ctx, state.ClusterID.ValueString(), true); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to restore cluster public access", err.Error())
	}
}

func (r *clusterPublicAccessResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID := strings.TrimSuffix(req.ID, "/public-access")
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), clusterID+"/public-access")...)
}
