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
	_ resource.Resource                = (*clientThemeAssignmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*clientThemeAssignmentResource)(nil)
	_ resource.ResourceWithImportState = (*clientThemeAssignmentResource)(nil)
)

type clientThemeAssignmentResource struct {
	client *skycloak.Client
}

// NewClientThemeAssignmentResource returns the skycloak_client_theme_assignment resource.
func NewClientThemeAssignmentResource() resource.Resource { return &clientThemeAssignmentResource{} }

type clientThemeAssignmentModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	RealmName types.String `tfsdk:"realm_name"`
	ClientID  types.String `tfsdk:"client_id"`
	Login     types.String `tfsdk:"login"`
}

func (r *clientThemeAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client_theme_assignment"
}

func (r *clientThemeAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Per-client login-theme override. Overrides the realm default for a single OIDC/SAML client.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/realm_name/client_id/theme`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: requiresReplace},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: requiresReplace},
			"client_id":  schema.StringAttribute{Required: true, MarkdownDescription: "OIDC/SAML client ID. Immutable.", PlanModifiers: requiresReplace},
			"login":      schema.StringAttribute{Optional: true, MarkdownDescription: "Custom theme ID to activate for this client's login page. Omit to use the realm default."},
		},
	}
}

func (r *clientThemeAssignmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *clientThemeAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clientThemeAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.client.SetClientThemeAssignment(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.ClientID.ValueString(), plan.Login.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to set client theme", err.Error())
		return
	}
	applyClientThemeToModel(a, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clientThemeAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clientThemeAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.client.GetClientThemeAssignment(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ClientID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read client theme", err.Error())
		return
	}
	applyClientThemeToModel(a, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clientThemeAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clientThemeAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.client.SetClientThemeAssignment(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.ClientID.ValueString(), plan.Login.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update client theme", err.Error())
		return
	}
	applyClientThemeToModel(a, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clientThemeAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clientThemeAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Reset the client override to the realm default.
	_, err := r.client.SetClientThemeAssignment(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ClientID.ValueString(), "")
	if err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to reset client theme", err.Error())
	}
}

func (r *clientThemeAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name/client_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("client_id"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[0]+"/"+parts[1]+"/"+parts[2]+"/theme")...)
}

func applyClientThemeToModel(a *skycloak.ClientThemeAssignment, m *clientThemeAssignmentModel) {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/" + m.RealmName.ValueString() + "/" + m.ClientID.ValueString() + "/theme")
	m.Login = optionalString(a.Login)
}
