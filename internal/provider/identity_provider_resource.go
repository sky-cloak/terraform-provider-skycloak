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
	ID                types.String   `tfsdk:"id"`
	ClusterID         types.String   `tfsdk:"cluster_id"`
	RealmName         types.String   `tfsdk:"realm_name"`
	ProviderID        types.String   `tfsdk:"provider_id"`
	Type              types.String   `tfsdk:"type"`
	DisplayName       types.String   `tfsdk:"display_name"`
	Enabled           types.Bool     `tfsdk:"enabled"`
	ClientID          types.String   `tfsdk:"client_id"`
	ClientSecret      types.String   `tfsdk:"client_secret"`
	ExternallyManaged types.Bool     `tfsdk:"externally_managed"`
	Config            idpConfigModel `tfsdk:"config"`
	CreatedAt         types.String   `tfsdk:"created_at"`
	UpdatedAt         types.String   `tfsdk:"updated_at"`
}

type idpConfigModel struct {
	ButtonText        types.String  `tfsdk:"button_text"`
	IconURL           types.String  `tfsdk:"icon_url"`
	SyncMode          types.String  `tfsdk:"sync_mode"`
	TrustEmail        types.Bool    `tfsdk:"trust_email"`
	AttributeMappings types.Map     `tfsdk:"attribute_mappings"`
	OIDC              *idpOIDCModel `tfsdk:"oidc"`
	LDAP              *idpLDAPModel `tfsdk:"ldap"`
	SAML              *idpSAMLModel `tfsdk:"saml"`
}

type idpOIDCModel struct {
	AuthorizationURL types.String `tfsdk:"authorization_url"`
	Issuer           types.String `tfsdk:"issuer"`
	LogoutURL        types.String `tfsdk:"logout_url"`
	TokenURL         types.String `tfsdk:"token_url"`
	UserinfoURL      types.String `tfsdk:"userinfo_url"`
}

type idpLDAPModel struct {
	BaseDN    types.String `tfsdk:"base_dn"`
	BindDN    types.String `tfsdk:"bind_dn"`
	ServerURL types.String `tfsdk:"server_url"`
}

type idpSAMLModel struct {
	EntityID    types.String `tfsdk:"entity_id"`
	MetadataURL types.String `tfsdk:"metadata_url"`
	MetadataXML types.String `tfsdk:"metadata_xml"`
	SSOURL      types.String `tfsdk:"sso_url"`
}

func (r *identityProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_identity_provider"
}

// optComp is an Optional+Computed string attribute (provider may normalize it).
func optComp(desc string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
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
			"display_name": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Display name shown on the login page."},
			"enabled":      schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether the provider is enabled. Defaults to `true`."},
			"client_id":    schema.StringAttribute{Optional: true, MarkdownDescription: "Upstream client/application ID."},
			"client_secret": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Upstream client secret. Write-only; never read back.",
			},
			"externally_managed": schema.BoolAttribute{Computed: true, MarkdownDescription: "True if the provider is managed outside Skycloak; such providers reject updates and deletes."},
			"config": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Provider configuration.",
				Attributes: map[string]schema.Attribute{
					"button_text": optComp("Login button label."),
					"icon_url":    optComp("Login button icon URL."),
					"sync_mode":   optComp("User profile sync mode (`IMPORT`, `LEGACY`, `FORCE`)."),
					"trust_email": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Trust email addresses from this provider."},
					"attribute_mappings": schema.MapAttribute{
						Optional:            true,
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Keycloak attribute → IdP claim mappings.",
					},
					"oidc": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: "OIDC endpoints (when `type` is `oidc`).",
						Attributes: map[string]schema.Attribute{
							"authorization_url": optComp("Authorization endpoint URL."),
							"issuer":            optComp("Issuer URL."),
							"logout_url":        optComp("Logout endpoint URL."),
							"token_url":         optComp("Token endpoint URL."),
							"userinfo_url":      optComp("Userinfo endpoint URL."),
						},
					},
					"ldap": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: "LDAP settings (when `type` is `ldap`).",
						Attributes: map[string]schema.Attribute{
							"base_dn":    optComp("Base DN."),
							"bind_dn":    optComp("Bind DN."),
							"server_url": optComp("LDAP server URL."),
						},
					},
					"saml": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: "SAML settings (when `type` is `saml`).",
						Attributes: map[string]schema.Attribute{
							"entity_id":    optComp("IdP entity ID."),
							"metadata_url": optComp("IdP metadata URL."),
							"metadata_xml": optComp("IdP metadata XML."),
							"sso_url":      optComp("Single sign-on service URL."),
						},
					},
				},
			},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
			"updated_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Last update timestamp."},
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
	cfg := skycloak.ProviderConfig{
		ButtonText:        m.Config.ButtonText.ValueString(),
		IconURL:           m.Config.IconURL.ValueString(),
		SyncMode:          m.Config.SyncMode.ValueString(),
		AttributeMappings: stringMapToMap(ctx, m.Config.AttributeMappings, &diags),
	}
	if !m.Config.TrustEmail.IsNull() && !m.Config.TrustEmail.IsUnknown() {
		v := m.Config.TrustEmail.ValueBool()
		cfg.TrustEmail = &v
	}
	if m.Config.OIDC != nil {
		cfg.OIDC = &skycloak.OIDCConfig{
			AuthorizationURL: m.Config.OIDC.AuthorizationURL.ValueString(), Issuer: m.Config.OIDC.Issuer.ValueString(),
			LogoutURL: m.Config.OIDC.LogoutURL.ValueString(), TokenURL: m.Config.OIDC.TokenURL.ValueString(), UserinfoURL: m.Config.OIDC.UserinfoURL.ValueString(),
		}
	}
	if m.Config.LDAP != nil {
		cfg.LDAP = &skycloak.LDAPConfig{BaseDN: m.Config.LDAP.BaseDN.ValueString(), BindDN: m.Config.LDAP.BindDN.ValueString(), ServerURL: m.Config.LDAP.ServerURL.ValueString()}
	}
	if m.Config.SAML != nil {
		cfg.SAML = &skycloak.SAMLConfig{EntityID: m.Config.SAML.EntityID.ValueString(), MetadataURL: m.Config.SAML.MetadataURL.ValueString(), MetadataXML: m.Config.SAML.MetadataXML.ValueString(), SSOURL: m.Config.SAML.SSOURL.ValueString()}
	}
	return skycloak.IdentityProvider{
		ProviderID:   m.ProviderID.ValueString(),
		Type:         m.Type.ValueString(),
		DisplayName:  m.DisplayName.ValueString(),
		Enabled:      m.Enabled.ValueBool(),
		ClientID:     m.ClientID.ValueString(),
		ClientSecret: m.ClientSecret.ValueString(),
		Config:       cfg,
	}, diags
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
	// client_secret is write-only; preserve whatever is in the model (state).
	m.ExternallyManaged = types.BoolValue(idp.ExternallyManaged)
	m.CreatedAt = types.StringValue(idp.CreatedAt)
	m.UpdatedAt = types.StringValue(idp.UpdatedAt)

	c := idp.Config
	m.Config.ButtonText = types.StringValue(c.ButtonText)
	m.Config.IconURL = types.StringValue(c.IconURL)
	m.Config.SyncMode = types.StringValue(c.SyncMode)
	m.Config.TrustEmail = types.BoolValue(c.TrustEmail != nil && *c.TrustEmail)
	m.Config.AttributeMappings = mapToStringMap(ctx, c.AttributeMappings, &diags)
	if c.OIDC != nil {
		m.Config.OIDC = &idpOIDCModel{
			AuthorizationURL: types.StringValue(c.OIDC.AuthorizationURL), Issuer: types.StringValue(c.OIDC.Issuer),
			LogoutURL: types.StringValue(c.OIDC.LogoutURL), TokenURL: types.StringValue(c.OIDC.TokenURL), UserinfoURL: types.StringValue(c.OIDC.UserinfoURL),
		}
	} else {
		m.Config.OIDC = nil
	}
	if c.LDAP != nil {
		m.Config.LDAP = &idpLDAPModel{BaseDN: types.StringValue(c.LDAP.BaseDN), BindDN: types.StringValue(c.LDAP.BindDN), ServerURL: types.StringValue(c.LDAP.ServerURL)}
	} else {
		m.Config.LDAP = nil
	}
	if c.SAML != nil {
		m.Config.SAML = &idpSAMLModel{EntityID: types.StringValue(c.SAML.EntityID), MetadataURL: types.StringValue(c.SAML.MetadataURL), MetadataXML: types.StringValue(c.SAML.MetadataXML), SSOURL: types.StringValue(c.SAML.SSOURL)}
	} else {
		m.Config.SAML = nil
	}
	return diags
}
