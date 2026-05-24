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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*realmResource)(nil)
	_ resource.ResourceWithConfigure   = (*realmResource)(nil)
	_ resource.ResourceWithImportState = (*realmResource)(nil)
)

type realmResource struct {
	client *skycloak.Client
}

// NewRealmResource returns the skycloak_realm resource.
func NewRealmResource() resource.Resource {
	return &realmResource{}
}

type realmModel struct {
	ID                          types.String `tfsdk:"id"`
	ClusterID                   types.String `tfsdk:"cluster_id"`
	Name                        types.String `tfsdk:"name"`
	DisplayName                 types.String `tfsdk:"display_name"`
	Enabled                     types.Bool   `tfsdk:"enabled"`
	SSLRequired                 types.String `tfsdk:"ssl_required"`
	RegistrationAllowed         types.Bool   `tfsdk:"registration_allowed"`
	RegistrationEmailAsUsername types.Bool   `tfsdk:"registration_email_as_username"`
	LoginWithEmailAllowed       types.Bool   `tfsdk:"login_with_email_allowed"`
	DuplicateEmailsAllowed      types.Bool   `tfsdk:"duplicate_emails_allowed"`
	CreatedAt                   types.String `tfsdk:"created_at"`
}

func (r *realmResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm"
}

func (r *realmResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Keycloak realm within a Skycloak cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/name`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the cluster the realm belongs to. Immutable.",
				PlanModifiers:       requiresReplace,
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Realm name (the Keycloak realm key). Immutable.",
				PlanModifiers:       requiresReplace,
			},
			"display_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Human-readable realm display name.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the realm is enabled. Defaults to `true`.",
			},
			"ssl_required": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("external"),
				MarkdownDescription: "SSL requirement: `external`, `all`, or `none`. Defaults to `external`.",
			},
			"registration_allowed": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Allow self-registration. Defaults to `false`.",
			},
			"registration_email_as_username": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Use email as username on registration. Defaults to `false`.",
			},
			"login_with_email_allowed": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Allow login with email. Defaults to `false`.",
			},
			"duplicate_emails_allowed": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Allow duplicate emails across users. Defaults to `false`.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp.",
			},
		},
	}
}

func (r *realmResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *realmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan realmModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ClusterID.ValueString()
	created, err := r.client.CreateRealm(ctx, clusterID, realmFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create realm", err.Error())
		return
	}
	applyRealmToModel(clusterID, created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ClusterID.ValueString()
	realm, err := r.client.GetRealm(ctx, clusterID, state.Name.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read realm", err.Error())
		return
	}
	applyRealmToModel(clusterID, realm, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *realmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan realmModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ClusterID.ValueString()
	updated, err := r.client.UpdateRealm(ctx, clusterID, plan.Name.ValueString(), realmFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update realm", err.Error())
		return
	}
	applyRealmToModel(clusterID, updated, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state realmModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteRealm(ctx, state.ClusterID.ValueString(), state.Name.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete realm", err.Error())
	}
}

func (r *realmResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID, name, ok := strings.Cut(req.ID, "/")
	if !ok || clusterID == "" || name == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func realmFromModel(m *realmModel) skycloak.Realm {
	return skycloak.Realm{
		Name:                        m.Name.ValueString(),
		DisplayName:                 m.DisplayName.ValueString(),
		Enabled:                     m.Enabled.ValueBool(),
		SSLRequired:                 m.SSLRequired.ValueString(),
		RegistrationAllowed:         m.RegistrationAllowed.ValueBool(),
		RegistrationEmailAsUsername: m.RegistrationEmailAsUsername.ValueBool(),
		LoginWithEmailAllowed:       m.LoginWithEmailAllowed.ValueBool(),
		DuplicateEmailsAllowed:      m.DuplicateEmailsAllowed.ValueBool(),
	}
}

func applyRealmToModel(clusterID string, rl *skycloak.Realm, m *realmModel) {
	m.ID = types.StringValue(clusterID + "/" + rl.Name)
	m.ClusterID = types.StringValue(clusterID)
	m.Name = types.StringValue(rl.Name)
	m.DisplayName = types.StringValue(rl.DisplayName)
	m.Enabled = types.BoolValue(rl.Enabled)
	m.SSLRequired = types.StringValue(rl.SSLRequired)
	m.RegistrationAllowed = types.BoolValue(rl.RegistrationAllowed)
	m.RegistrationEmailAsUsername = types.BoolValue(rl.RegistrationEmailAsUsername)
	m.LoginWithEmailAllowed = types.BoolValue(rl.LoginWithEmailAllowed)
	m.DuplicateEmailsAllowed = types.BoolValue(rl.DuplicateEmailsAllowed)
	m.CreatedAt = types.StringValue(rl.CreatedAt)
}
