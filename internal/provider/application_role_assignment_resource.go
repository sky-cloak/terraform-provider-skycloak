package provider

import (
	"context"
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
	_ resource.Resource                = (*applicationRoleAssignmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*applicationRoleAssignmentResource)(nil)
	_ resource.ResourceWithImportState = (*applicationRoleAssignmentResource)(nil)
)

type applicationRoleAssignmentResource struct{ client *skycloak.Client }

// NewApplicationRoleAssignmentResource returns the skycloak_application_role_assignment resource.
func NewApplicationRoleAssignmentResource() resource.Resource {
	return &applicationRoleAssignmentResource{}
}

type applicationRoleAssignmentModel struct {
	ID           types.String `tfsdk:"id"`
	ClusterID    types.String `tfsdk:"cluster_id"`
	RealmName    types.String `tfsdk:"realm_name"`
	ClientID     types.String `tfsdk:"client_id"`
	RoleName     types.String `tfsdk:"role_name"`
	RoleClientID types.String `tfsdk:"role_client_id"`
}

func (r *applicationRoleAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_role_assignment"
}

func (r *applicationRoleAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Grants a role to an application's service account. One resource per (client, role) edge.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: rrStr},
			"client_id":  schema.StringAttribute{Required: true, MarkdownDescription: "Application client ID receiving the role. Immutable.", PlanModifiers: rrStr},
			"role_name":  schema.StringAttribute{Required: true, MarkdownDescription: "Role name to assign. Immutable.", PlanModifiers: rrStr},
			"role_client_id": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Client ID owning the role, for a client role. Omit for a realm role. Immutable.", PlanModifiers: rrStr,
			},
		},
	}
}

func (r *applicationRoleAssignmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = providerClient(req.ProviderData, &resp.Diagnostics)
}

func (r *applicationRoleAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationRoleAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.AssignApplicationRole(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.ClientID.ValueString(), plan.RoleName.ValueString(), plan.RoleClientID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to assign application role", err.Error())
		return
	}
	plan.ID = types.StringValue(strings.Join([]string{plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.ClientID.ValueString(), plan.RoleName.ValueString()}, "/"))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationRoleAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationRoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	roles, err := r.client.ListApplicationRoles(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ClientID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read application roles", err.Error())
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

func (r *applicationRoleAssignmentResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes are RequiresReplace; Update is never called.
}

func (r *applicationRoleAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationRoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RemoveApplicationRole(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ClientID.ValueString(), state.RoleName.ValueString(), state.RoleClientID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to remove application role", err.Error())
	}
}

func (r *applicationRoleAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 4 {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name/client_id/role_name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("client_id"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_name"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
