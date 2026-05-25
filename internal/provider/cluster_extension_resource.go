package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*clusterExtensionResource)(nil)
	_ resource.ResourceWithConfigure   = (*clusterExtensionResource)(nil)
	_ resource.ResourceWithImportState = (*clusterExtensionResource)(nil)
)

type clusterExtensionResource struct {
	client *skycloak.Client
}

// NewClusterExtensionResource returns the skycloak_cluster_extension resource.
func NewClusterExtensionResource() resource.Resource { return &clusterExtensionResource{} }

type clusterExtensionModel struct {
	ID               types.String `tfsdk:"id"`
	ClusterID        types.String `tfsdk:"cluster_id"`
	ExtensionID      types.String `tfsdk:"extension_id"`
	Parameters       types.Map    `tfsdk:"parameters"`
	ExtensionName    types.String `tfsdk:"extension_name"`
	Source           types.String `tfsdk:"source"`
	InstalledVersion types.String `tfsdk:"installed_version"`
	AvailableVersion types.String `tfsdk:"available_version"`
	Status           types.String `tfsdk:"status"`
	UpgradeAvailable types.Bool   `tfsdk:"upgrade_available"`
	InstalledAt      types.String `tfsdk:"installed_at"`
}

func (r *clusterExtensionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_extension"
}

func (r *clusterExtensionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Installs a marketplace extension on a cluster. Installation is asynchronous; the `status` reflects provisioning progress. Changing `extension_id` or `parameters` reinstalls the extension.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/extension_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id":   schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: requiresReplace},
			"extension_id": schema.StringAttribute{Required: true, MarkdownDescription: "Catalog extension ID (see the `skycloak_extensions` data source). Immutable.", PlanModifiers: requiresReplace},
			"parameters": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				Sensitive:           true,
				MarkdownDescription: "Parameter values keyed by parameter `key`. Write-only (sensitive values are never read back). Changing this reinstalls the extension.",
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"extension_name":    schema.StringAttribute{Computed: true, MarkdownDescription: "Extension display name."},
			"source":            schema.StringAttribute{Computed: true, MarkdownDescription: "Extension source (`marketplace`, `custom`)."},
			"installed_version": schema.StringAttribute{Computed: true, MarkdownDescription: "Version currently running on the cluster."},
			"available_version": schema.StringAttribute{Computed: true, MarkdownDescription: "Latest available version."},
			"status":            schema.StringAttribute{Computed: true, MarkdownDescription: "Installation status."},
			"upgrade_available": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a newer version is available."},
			"installed_at":      schema.StringAttribute{Computed: true, MarkdownDescription: "Install timestamp (RFC 3339)."},
		},
	}
}

func (r *clusterExtensionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *clusterExtensionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterExtensionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := stringMapToMap(ctx, plan.Parameters, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	ext, err := r.client.InstallExtension(ctx, plan.ClusterID.ValueString(), plan.ExtensionID.ValueString(), params)
	if err != nil {
		resp.Diagnostics.AddError("Unable to install extension", err.Error())
		return
	}
	applyClusterExtensionToModel(ext, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterExtensionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterExtensionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ext, err := r.client.GetClusterExtension(ctx, state.ClusterID.ValueString(), state.ExtensionID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read cluster extension", err.Error())
		return
	}
	applyClusterExtensionToModel(ext, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only runs for in-place attribute changes. All configurable attributes
// are RequiresReplace, so this re-reads current state for safety.
func (r *clusterExtensionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterExtensionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ext, err := r.client.GetClusterExtension(ctx, plan.ClusterID.ValueString(), plan.ExtensionID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cluster extension", err.Error())
		return
	}
	applyClusterExtensionToModel(ext, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterExtensionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterExtensionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UninstallExtension(ctx, state.ClusterID.ValueString(), state.ExtensionID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to uninstall extension", err.Error())
	}
}

func (r *clusterExtensionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/extension_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("extension_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[0]+"/"+parts[1])...)
}

// applyClusterExtensionToModel copies API fields into the model. The write-only
// parameters map already in the model is left untouched, since sensitive values
// are never returned.
func applyClusterExtensionToModel(e *skycloak.ClusterExtension, m *clusterExtensionModel) {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/" + m.ExtensionID.ValueString())
	m.ExtensionName = types.StringValue(e.ExtensionName)
	m.Source = types.StringValue(e.Source)
	m.InstalledVersion = types.StringValue(e.InstalledVersion)
	m.AvailableVersion = optionalString(e.AvailableVersion)
	m.Status = types.StringValue(e.Status)
	m.UpgradeAvailable = types.BoolValue(e.UpgradeAvailable)
	m.InstalledAt = types.StringValue(e.InstalledAt)
}
