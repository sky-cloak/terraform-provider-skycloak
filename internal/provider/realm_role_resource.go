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
	_ resource.Resource                = (*realmRoleResource)(nil)
	_ resource.ResourceWithConfigure   = (*realmRoleResource)(nil)
	_ resource.ResourceWithImportState = (*realmRoleResource)(nil)
)

type realmRoleResource struct {
	client *skycloak.Client
}

// NewRealmRoleResource returns the skycloak_realm_role resource.
func NewRealmRoleResource() resource.Resource { return &realmRoleResource{} }

type realmRoleModel struct {
	ID          types.String `tfsdk:"id"`
	ClusterID   types.String `tfsdk:"cluster_id"`
	RealmName   types.String `tfsdk:"realm_name"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Composite   types.Bool   `tfsdk:"composite"`
	ClientRole  types.Bool   `tfsdk:"client_role"`
}

func (r *realmRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_role"
}

func (r *realmRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A realm-scoped role. Assign it to users with `skycloak_realm_user_role_assignment`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/realm_name/role_name`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id":  schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"realm_name":  schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: rrStr},
			"name":        schema.StringAttribute{Required: true, MarkdownDescription: "Role name. Immutable (renaming recreates the role).", PlanModifiers: rrStr},
			"description": schema.StringAttribute{Optional: true, MarkdownDescription: "Human-friendly role description."},
			"composite":   schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this role is composed from other roles."},
			"client_role": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is a client-scoped role."},
		},
	}
}

func (r *realmRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *realmRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan realmRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, err := r.client.CreateRealmRole(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.Name.ValueString(), plan.Description.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create realm role", err.Error())
		return
	}
	applyRealmRoleToModel(role, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, err := r.client.GetRealmRole(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.Name.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read realm role", err.Error())
		return
	}
	applyRealmRoleToModel(role, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *realmRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan realmRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, err := r.client.UpdateRealmRole(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.Name.ValueString(), plan.Description.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update realm role", err.Error())
		return
	}
	applyRealmRoleToModel(role, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state realmRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRealmRole(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.Name.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete realm role", err.Error())
	}
}

func (r *realmRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name/role_name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[2])...)
}

func applyRealmRoleToModel(role *skycloak.RealmRole, m *realmRoleModel) {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/" + m.RealmName.ValueString() + "/" + role.Name)
	m.Name = types.StringValue(role.Name)
	m.Description = optionalString(role.Description)
	m.Composite = types.BoolValue(role.Composite)
	m.ClientRole = types.BoolValue(role.ClientRole)
}
