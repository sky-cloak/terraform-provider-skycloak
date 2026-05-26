package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ datasource.DataSource              = (*oidcDiscoveryDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*oidcDiscoveryDataSource)(nil)
)

type oidcDiscoveryDataSource struct{ client *skycloak.Client }

// NewOIDCDiscoveryDataSource returns the skycloak_oidc_discovery data source.
func NewOIDCDiscoveryDataSource() datasource.DataSource { return &oidcDiscoveryDataSource{} }

type oidcDiscoveryModel struct {
	IssuerURL             types.String `tfsdk:"issuer_url"`
	Issuer                types.String `tfsdk:"issuer"`
	AuthorizationEndpoint types.String `tfsdk:"authorization_endpoint"`
	TokenEndpoint         types.String `tfsdk:"token_endpoint"`
	UserinfoEndpoint      types.String `tfsdk:"userinfo_endpoint"`
	JwksURI               types.String `tfsdk:"jwks_uri"`
	ScopesSupported       types.List   `tfsdk:"scopes_supported"`
}

func (d *oidcDiscoveryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oidc_discovery"
}

func (d *oidcDiscoveryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Resolves an OIDC issuer's discovery document. Use the returned endpoints to pre-fill a `skycloak_identity_provider`.",
		Attributes: map[string]schema.Attribute{
			"issuer_url":             schema.StringAttribute{Required: true, MarkdownDescription: "OIDC issuer URL. The `/.well-known/openid-configuration` suffix is appended automatically."},
			"issuer":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Canonical issuer identifier."},
			"authorization_endpoint": schema.StringAttribute{Computed: true, MarkdownDescription: "Authorization endpoint."},
			"token_endpoint":         schema.StringAttribute{Computed: true, MarkdownDescription: "Token endpoint."},
			"userinfo_endpoint":      schema.StringAttribute{Computed: true, MarkdownDescription: "UserInfo endpoint."},
			"jwks_uri":               schema.StringAttribute{Computed: true, MarkdownDescription: "JWKS URI."},
			"scopes_supported":       schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Scopes advertised by the provider."},
		},
	}
}

func (d *oidcDiscoveryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *oidcDiscoveryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg oidcDiscoveryModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	doc, err := d.client.DiscoverOIDC(ctx, cfg.IssuerURL.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to discover OIDC endpoints", err.Error())
		return
	}
	cfg.Issuer = types.StringValue(doc.Issuer)
	cfg.AuthorizationEndpoint = types.StringValue(doc.AuthorizationEndpoint)
	cfg.TokenEndpoint = types.StringValue(doc.TokenEndpoint)
	cfg.UserinfoEndpoint = optionalString(doc.UserinfoEndpoint)
	cfg.JwksURI = optionalString(doc.JwksURI)
	cfg.ScopesSupported = sliceToStringList(ctx, doc.ScopesSupported, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
