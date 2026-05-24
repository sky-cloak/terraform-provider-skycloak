package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                = (*identityProviderResource)(nil)
	_ resource.ResourceWithConfigure   = (*identityProviderResource)(nil)
	_ resource.ResourceWithImportState = (*identityProviderResource)(nil)
)

type identityProviderResource struct {
	client *skycloak.Client
}

// NewIdentityProviderResource returns the skycloak_identity_provider resource.
func NewIdentityProviderResource() resource.Resource {
	return &identityProviderResource{}
}

type identityProviderModel struct {
	ID                types.String `tfsdk:"id"`
	ClusterID         types.String `tfsdk:"cluster_id"`
	RealmName         types.String `tfsdk:"realm_name"`
	ProviderID        types.String `tfsdk:"provider_id"`
	Type              types.String `tfsdk:"type"`
	DisplayName       types.String `tfsdk:"display_name"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	ClientID          types.String `tfsdk:"client_id"`
	Config            types.Map    `tfsdk:"config"`
	ExternallyManaged types.Bool   `tfsdk:"externally_managed"`
	CreatedAt         types.String `tfsdk:"created_at"`
	UpdatedAt         types.String `tfsdk:"updated_at"`
}

func (r *identityProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_identity_provider"
}

func (r *identityProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An identity provider (SSO connection) in a Skycloak realm.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/realm_name/provider_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id":  schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: requiresReplace},
			"realm_name":  schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: requiresReplace},
			"provider_id": schema.StringAttribute{Required: true, MarkdownDescription: "Unique provider alias within the realm. Immutable.", PlanModifiers: requiresReplace},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Provider kind: `oidc`, `saml`, or `ldap`. Immutable.",
				PlanModifiers:       requiresReplace,
			},
			"display_name": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Display name shown on the login page.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"enabled":      schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether the provider is enabled. Defaults to `true`."},
			"client_id":    schema.StringAttribute{Optional: true, MarkdownDescription: "Upstream client/app ID, where applicable."},
			"config": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Provider-specific configuration (e.g. `authorizationUrl`, `tokenUrl`, `clientSecret`).",
			},
			"externally_managed": schema.BoolAttribute{Computed: true, MarkdownDescription: "True if the provider is managed outside Skycloak; such providers reject updates and deletes."},
			"created_at":         schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
			"updated_at":         schema.StringAttribute{Computed: true, MarkdownDescription: "Last update timestamp."},
		},
	}
}

func (r *identityProviderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *identityProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan identityProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idp, diags := idpFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateIdentityProvider(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), idp)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create identity provider", err.Error())
		return
	}
	resp.Diagnostics.Append(applyIdpToModel(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), created, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *identityProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state identityProviderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idp, err := r.client.GetIdentityProvider(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ProviderID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read identity provider", err.Error())
		return
	}
	resp.Diagnostics.Append(applyIdpToModel(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), idp, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *identityProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan identityProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idp, diags := idpFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateIdentityProvider(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.ProviderID.ValueString(), idp)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update identity provider", err.Error())
		return
	}
	resp.Diagnostics.Append(applyIdpToModel(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), updated, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *identityProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state identityProviderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteIdentityProvider(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ProviderID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete identity provider", err.Error())
	}
}

func (r *identityProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name/provider_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("provider_id"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func idpFromModel(ctx context.Context, m *identityProviderModel) (skycloak.IdentityProvider, diag.Diagnostics) {
	var diags diag.Diagnostics
	idp := skycloak.IdentityProvider{
		ProviderID:  m.ProviderID.ValueString(),
		Type:        m.Type.ValueString(),
		DisplayName: m.DisplayName.ValueString(),
		Enabled:     m.Enabled.ValueBool(),
		ClientID:    m.ClientID.ValueString(),
		Config:      stringMapToMap(ctx, m.Config, &diags),
	}
	return idp, diags
}

func applyIdpToModel(ctx context.Context, clusterID, realm string, idp *skycloak.IdentityProvider, m *identityProviderModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(clusterID + "/" + realm + "/" + idp.ProviderID)
	m.ClusterID = types.StringValue(clusterID)
	m.RealmName = types.StringValue(realm)
	m.ProviderID = types.StringValue(idp.ProviderID)
	m.Type = types.StringValue(idp.Type)
	m.DisplayName = types.StringValue(idp.DisplayName)
	m.Enabled = types.BoolValue(idp.Enabled)
	m.ClientID = optionalString(idp.ClientID)
	m.Config = mapToStringMap(ctx, idp.Config, &diags)
	m.ExternallyManaged = types.BoolValue(idp.ExternallyManaged)
	m.CreatedAt = types.StringValue(idp.CreatedAt)
	m.UpdatedAt = types.StringValue(idp.UpdatedAt)
	return diags
}
