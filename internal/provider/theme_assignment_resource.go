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
	_ resource.Resource                = (*themeAssignmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*themeAssignmentResource)(nil)
	_ resource.ResourceWithImportState = (*themeAssignmentResource)(nil)
)

type themeAssignmentResource struct {
	client *skycloak.Client
}

// NewThemeAssignmentResource returns the skycloak_theme_assignment resource.
func NewThemeAssignmentResource() resource.Resource { return &themeAssignmentResource{} }

type themeAssignmentModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	RealmName types.String `tfsdk:"realm_name"`
	Login     types.String `tfsdk:"login"`
	Account   types.String `tfsdk:"account"`
	Admin     types.String `tfsdk:"admin"`
	Email     types.String `tfsdk:"email"`
}

func (r *themeAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_theme_assignment"
}

func (r *themeAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	themeAttr := func(t string) schema.StringAttribute {
		return schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: fmt.Sprintf("Custom theme ID to activate for the %s. Omit to use Keycloak's built-in default.", t),
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Realm-level custom theme assignment per Keycloak theme type. A singleton per realm; managing this resource asserts the full assignment.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/realm_name/theme_assignment`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: requiresReplace},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: requiresReplace},
			"login":      themeAttr("login page"),
			"account":    themeAttr("account console"),
			"admin":      themeAttr("admin console"),
			"email":      themeAttr("email templates"),
		},
	}
}

func (r *themeAssignmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *themeAssignmentResource) set(ctx context.Context, plan *themeAssignmentModel) (*skycloak.ThemeAssignment, error) {
	return r.client.SetThemeAssignment(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), skycloak.ThemeAssignment{
		Login:   plan.Login.ValueString(),
		Account: plan.Account.ValueString(),
		Admin:   plan.Admin.ValueString(),
		Email:   plan.Email.ValueString(),
	})
}

func (r *themeAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan themeAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Unable to set theme assignment", err.Error())
		return
	}
	applyThemeAssignmentToModel(a, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *themeAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state themeAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.client.GetThemeAssignment(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read theme assignment", err.Error())
		return
	}
	applyThemeAssignmentToModel(a, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *themeAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan themeAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.set(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update theme assignment", err.Error())
		return
	}
	applyThemeAssignmentToModel(a, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *themeAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state themeAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Reset every theme type to Keycloak's built-in default.
	_, err := r.client.SetThemeAssignment(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), skycloak.ThemeAssignment{})
	if err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to reset theme assignment", err.Error())
	}
}

func (r *themeAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[0]+"/"+parts[1]+"/theme_assignment")...)
}

func applyThemeAssignmentToModel(a *skycloak.ThemeAssignment, m *themeAssignmentModel) {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/" + m.RealmName.ValueString() + "/theme_assignment")
	m.Login = optionalString(a.Login)
	m.Account = optionalString(a.Account)
	m.Admin = optionalString(a.Admin)
	m.Email = optionalString(a.Email)
}
