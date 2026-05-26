// Package provider implements the Skycloak Terraform provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// Ensure skycloakProvider satisfies the provider.Provider interface.
var _ provider.Provider = (*skycloakProvider)(nil)

type skycloakProvider struct {
	version string
}

// New returns a factory for the Skycloak provider.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &skycloakProvider{version: version}
	}
}

func (p *skycloakProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "skycloak"
	resp.Version = p.version
}

type providerModel struct {
	Endpoint   types.String `tfsdk:"endpoint"`
	APIKey     types.String `tfsdk:"api_key"`
	APIVersion types.String `tfsdk:"api_version"`
}

func (p *skycloakProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage your Skycloak managed-Keycloak environment as code.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Public API base URL. Defaults to `https://api.skycloak.io` (env `SKYCLOAK_ENDPOINT`).",
			},
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Skycloak API key (`sk_sc_{env}_..._...`). Prefer the `SKYCLOAK_API_KEY` env var. Minted in the dashboard.",
			},
			"api_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Date-based API version, e.g. `2026-03-01` (env `SKYCLOAK_API_VERSION`). Defaults to the current version.",
			},
		},
	}
}

func (p *skycloakProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := firstNonEmpty(cfg.Endpoint.ValueString(), os.Getenv("SKYCLOAK_ENDPOINT"))
	apiKey := firstNonEmpty(cfg.APIKey.ValueString(), os.Getenv("SKYCLOAK_API_KEY"))
	apiVersion := firstNonEmpty(cfg.APIVersion.ValueString(), os.Getenv("SKYCLOAK_API_VERSION"))

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing Skycloak API key",
			"Set the api_key provider argument or the SKYCLOAK_API_KEY environment variable. "+
				"Keys are minted in the Skycloak dashboard (the public API cannot yet issue keys).",
		)
		return
	}

	client := skycloak.New(endpoint, apiKey, apiVersion, skycloak.WithUserAgent("terraform-provider-skycloak/"+p.version))
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *skycloakProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewClusterResource,
		NewRealmResource,
		NewApplicationResource,
		NewApplicationSecretResource,
		NewIdentityProviderResource,
		NewSMTPResource,
		NewDomainResource,
		NewDomainRouteResource,
		NewLoginBrandingResource,
		NewEmailBrandingResource,
		NewThemeAssignmentResource,
		NewClientThemeAssignmentResource,
		NewClusterExtensionResource,
		NewExportResource,
		NewThemeResource,
		NewCustomExtensionResource,
		NewRealmRoleResource,
		NewRealmGroupResource,
		NewRealmUserResource,
		NewRealmUserRoleAssignmentResource,
		NewRealmGroupMembershipResource,
	}
}

func (p *skycloakProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterDataSource,
		NewClusterLocationsDataSource,
		NewClusterTypesDataSource,
		NewClusterFeaturesDataSource,
		NewThemesDataSource,
		NewExtensionsDataSource,
		NewRealmRolesDataSource,
		NewRealmGroupsDataSource,
		NewRealmUsersDataSource,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
