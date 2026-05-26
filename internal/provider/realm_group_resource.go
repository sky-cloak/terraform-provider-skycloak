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
	_ resource.Resource                = (*realmGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*realmGroupResource)(nil)
	_ resource.ResourceWithImportState = (*realmGroupResource)(nil)
)

type realmGroupResource struct {
	client *skycloak.Client
}

// NewRealmGroupResource returns the skycloak_realm_group resource.
func NewRealmGroupResource() resource.Resource { return &realmGroupResource{} }

type realmGroupModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	RealmName types.String `tfsdk:"realm_name"`
	Name      types.String `tfsdk:"name"`
	ParentID  types.String `tfsdk:"parent_id"`
	Path      types.String `tfsdk:"path"`
}

func (r *realmGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_group"
}

func (r *realmGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A realm group. Add users with `skycloak_realm_group_membership`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Group ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: rrStr},
			"name":       schema.StringAttribute{Required: true, MarkdownDescription: "Group name (leaf segment of the path)."},
			"parent_id":  schema.StringAttribute{Optional: true, MarkdownDescription: "Parent group ID for a nested group. Immutable.", PlanModifiers: rrStr},
			"path":       schema.StringAttribute{Computed: true, MarkdownDescription: "Canonical group path."},
		},
	}
}

func (r *realmGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *realmGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan realmGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group, err := r.client.CreateRealmGroup(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.Name.ValueString(), plan.ParentID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create realm group", err.Error())
		return
	}
	applyRealmGroupToModel(group, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group, err := r.client.GetRealmGroup(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read realm group", err.Error())
		return
	}
	applyRealmGroupToModel(group, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *realmGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan realmGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group, err := r.client.UpdateRealmGroup(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.ID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update realm group", err.Error())
		return
	}
	applyRealmGroupToModel(group, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state realmGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRealmGroup(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete realm group", err.Error())
	}
}

func (r *realmGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name/group_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}

// applyRealmGroupToModel copies API fields into the model, leaving parent_id
// (a create-only input the API does not echo back) untouched.
func applyRealmGroupToModel(g *skycloak.RealmGroup, m *realmGroupModel) {
	m.ID = types.StringValue(g.ID)
	m.Name = types.StringValue(g.Name)
	m.Path = types.StringValue(g.Path)
}
