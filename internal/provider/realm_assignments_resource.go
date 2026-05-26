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

func providerClient(data any, diags interface{ AddError(string, string) }) *skycloak.Client {
	if data == nil {
		return nil
	}
	client, ok := data.(*skycloak.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *skycloak.Client, got %T", data))
		return nil
	}
	return client
}

// ---- skycloak_realm_user_role_assignment ----

var (
	_ resource.Resource                = (*realmUserRoleAssignmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*realmUserRoleAssignmentResource)(nil)
	_ resource.ResourceWithImportState = (*realmUserRoleAssignmentResource)(nil)
)

type realmUserRoleAssignmentResource struct{ client *skycloak.Client }

// NewRealmUserRoleAssignmentResource returns the skycloak_realm_user_role_assignment resource.
func NewRealmUserRoleAssignmentResource() resource.Resource {
	return &realmUserRoleAssignmentResource{}
}

type realmUserRoleAssignmentModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	RealmName types.String `tfsdk:"realm_name"`
	UserID    types.String `tfsdk:"user_id"`
	RoleName  types.String `tfsdk:"role_name"`
}

func (r *realmUserRoleAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_user_role_assignment"
}

func (r *realmUserRoleAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assigns a single realm role to a user. One resource per (user, role) edge.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: rrStr},
			"user_id":    schema.StringAttribute{Required: true, MarkdownDescription: "User ID. Immutable.", PlanModifiers: rrStr},
			"role_name":  schema.StringAttribute{Required: true, MarkdownDescription: "Realm role name. Immutable.", PlanModifiers: rrStr},
		},
	}
}

func (r *realmUserRoleAssignmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = providerClient(req.ProviderData, &resp.Diagnostics)
}

func (r *realmUserRoleAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan realmUserRoleAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.AssignRealmUserRole(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.UserID.ValueString(), plan.RoleName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to assign role", err.Error())
		return
	}
	plan.ID = types.StringValue(strings.Join([]string{plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.UserID.ValueString(), plan.RoleName.ValueString()}, "/"))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmUserRoleAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmUserRoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	roles, err := r.client.ListRealmUserRoles(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.UserID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read user roles", err.Error())
		return
	}
	for _, role := range roles {
		if role.Name == state.RoleName.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *realmUserRoleAssignmentResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are RequiresReplace; Update is never called.
}

func (r *realmUserRoleAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state realmUserRoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RemoveRealmUserRole(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.UserID.ValueString(), state.RoleName.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to remove role", err.Error())
	}
}

func (r *realmUserRoleAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 4 {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name/user_id/role_name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_name"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// ---- skycloak_realm_group_membership ----

var (
	_ resource.Resource                = (*realmGroupMembershipResource)(nil)
	_ resource.ResourceWithConfigure   = (*realmGroupMembershipResource)(nil)
	_ resource.ResourceWithImportState = (*realmGroupMembershipResource)(nil)
)

type realmGroupMembershipResource struct{ client *skycloak.Client }

// NewRealmGroupMembershipResource returns the skycloak_realm_group_membership resource.
func NewRealmGroupMembershipResource() resource.Resource { return &realmGroupMembershipResource{} }

type realmGroupMembershipModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	RealmName types.String `tfsdk:"realm_name"`
	UserID    types.String `tfsdk:"user_id"`
	GroupID   types.String `tfsdk:"group_id"`
}

func (r *realmGroupMembershipResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_group_membership"
}

func (r *realmGroupMembershipResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Adds a user to a realm group. One resource per (user, group) edge.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: rrStr},
			"user_id":    schema.StringAttribute{Required: true, MarkdownDescription: "User ID. Immutable.", PlanModifiers: rrStr},
			"group_id":   schema.StringAttribute{Required: true, MarkdownDescription: "Group ID. Immutable.", PlanModifiers: rrStr},
		},
	}
}

func (r *realmGroupMembershipResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = providerClient(req.ProviderData, &resp.Diagnostics)
}

func (r *realmGroupMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan realmGroupMembershipModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.AddRealmUserToGroup(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.UserID.ValueString(), plan.GroupID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to add user to group", err.Error())
		return
	}
	plan.ID = types.StringValue(strings.Join([]string{plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.UserID.ValueString(), plan.GroupID.ValueString()}, "/"))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmGroupMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmGroupMembershipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groups, err := r.client.ListRealmUserGroups(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.UserID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read user groups", err.Error())
		return
	}
	for _, g := range groups {
		if g.ID == state.GroupID.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *realmGroupMembershipResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are RequiresReplace; Update is never called.
}

func (r *realmGroupMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state realmGroupMembershipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RemoveRealmUserFromGroup(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.UserID.ValueString(), state.GroupID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to remove user from group", err.Error())
	}
}

func (r *realmGroupMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 4 {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name/user_id/group_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
