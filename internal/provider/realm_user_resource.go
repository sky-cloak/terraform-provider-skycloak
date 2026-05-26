package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*realmUserResource)(nil)
	_ resource.ResourceWithConfigure   = (*realmUserResource)(nil)
	_ resource.ResourceWithImportState = (*realmUserResource)(nil)
)

type realmUserResource struct {
	client *skycloak.Client
}

// NewRealmUserResource returns the skycloak_realm_user resource.
func NewRealmUserResource() resource.Resource { return &realmUserResource{} }

type realmUserModel struct {
	ID                types.String `tfsdk:"id"`
	ClusterID         types.String `tfsdk:"cluster_id"`
	RealmName         types.String `tfsdk:"realm_name"`
	Username          types.String `tfsdk:"username"`
	Email             types.String `tfsdk:"email"`
	FirstName         types.String `tfsdk:"first_name"`
	LastName          types.String `tfsdk:"last_name"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	EmailVerified     types.Bool   `tfsdk:"email_verified"`
	TemporaryPassword types.String `tfsdk:"temporary_password"`
	CreatedAt         types.String `tfsdk:"created_at"`
	LastLoginAt       types.String `tfsdk:"last_login_at"`
}

func (r *realmUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_user"
}

func (r *realmUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A realm user. Grant roles with `skycloak_realm_user_role_assignment` and group membership with `skycloak_realm_group_membership`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "User ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: rrStr},
			"username":   schema.StringAttribute{Required: true, MarkdownDescription: "Username. Immutable.", PlanModifiers: rrStr},
			"email":      schema.StringAttribute{Required: true, MarkdownDescription: "Email address."},
			"first_name": schema.StringAttribute{Optional: true, MarkdownDescription: "First name."},
			"last_name":  schema.StringAttribute{Optional: true, MarkdownDescription: "Last name."},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Whether the user can sign in. Defaults to `true`.",
			},
			"email_verified": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Whether the email address is verified. Defaults to `false`.",
			},
			"temporary_password": schema.StringAttribute{
				Required: true, Sensitive: true,
				MarkdownDescription: "Initial password (min 8 chars). Write-only; changing it recreates the user (there is no password-update endpoint).",
				PlanModifiers:       rrStr,
			},
			"created_at":    schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp (RFC 3339)."},
			"last_login_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Most recent sign-in (RFC 3339)."},
		},
	}
}

func (r *realmUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (m *realmUserModel) toRequest() skycloak.CreateRealmUserRequest {
	return skycloak.CreateRealmUserRequest{
		Username: m.Username.ValueString(), Email: m.Email.ValueString(),
		FirstName: m.FirstName.ValueString(), LastName: m.LastName.ValueString(),
		Enabled: m.Enabled.ValueBool(), TemporaryPassword: m.TemporaryPassword.ValueString(),
	}
}

func (r *realmUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan realmUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	user, err := r.client.CreateRealmUser(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.toRequest())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create realm user", err.Error())
		return
	}
	applyRealmUserToModel(user, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	user, err := r.client.GetRealmUser(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read realm user", err.Error())
		return
	}
	applyRealmUserToModel(user, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *realmUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan realmUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	user, err := r.client.UpdateRealmUser(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.ID.ValueString(), plan.toRequest(), plan.EmailVerified.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update realm user", err.Error())
		return
	}
	applyRealmUserToModel(user, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state realmUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRealmUser(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete realm user", err.Error())
	}
}

func (r *realmUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name/user_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}

// applyRealmUserToModel copies API fields into the model, leaving the write-only
// temporary_password untouched.
func applyRealmUserToModel(u *skycloak.RealmUser, m *realmUserModel) {
	m.ID = types.StringValue(u.ID)
	m.Username = types.StringValue(u.Username)
	m.Email = types.StringValue(u.Email)
	m.FirstName = optionalString(u.FirstName)
	m.LastName = optionalString(u.LastName)
	m.Enabled = types.BoolValue(u.Enabled)
	m.EmailVerified = types.BoolValue(u.EmailVerified)
	m.CreatedAt = optionalString(u.CreatedAt)
	m.LastLoginAt = optionalString(u.LastLoginAt)
}
