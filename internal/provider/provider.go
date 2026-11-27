// Package provider implements the Skycloak Terraform provider.
package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// Ensure skycloakProvider satisfies the provider interfaces.
var (
	_ provider.Provider            = (*skycloakProvider)(nil)
	_ provider.ProviderWithActions = (*skycloakProvider)(nil)
)

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
	Endpoint         types.String `tfsdk:"endpoint"`
	APIKey           types.String `tfsdk:"api_key"`
	APIVersion       types.String `tfsdk:"api_version"`
	PlatformEndpoint types.String `tfsdk:"platform_endpoint"`
	PlatformAPIKey   types.String `tfsdk:"platform_api_key"`
}

func (p *skycloakProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage your Skycloak managed Keycloak and Platform projects as code. Each product has its own API key; set the key for the products you use.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Public API base URL. Defaults to `https://api.skycloak.io` (env `SKYCLOAK_ENDPOINT`).",
			},
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Skycloak API key (`sk_sc_{env}_..._...`). Prefer the `SKYCLOAK_API_KEY` env var. Minted in the dashboard. Needed only by managed Keycloak resources, data sources and actions.",
			},
			"api_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Date-based API version, e.g. `2026-06-01.beta` (env `SKYCLOAK_API_VERSION`). Defaults to the version this provider release was built against.",
			},
			"platform_endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Platform API base URL. Defaults to `https://api.skycloak.io` (env `SKYCLOAK_PLATFORM_ENDPOINT`).",
			},
			"platform_api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Skycloak Platform API key. Prefer the `SKYCLOAK_PLATFORM_API_KEY` env var. Needed only by Platform resources and data sources.",
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
	platformEndpoint := firstNonEmpty(cfg.PlatformEndpoint.ValueString(), os.Getenv("SKYCLOAK_PLATFORM_ENDPOINT"))
	platformAPIKey := firstNonEmpty(cfg.PlatformAPIKey.ValueString(), os.Getenv("SKYCLOAK_PLATFORM_API_KEY"))

	if apiVersion != "" && apiVersion != skycloak.DefaultAPIVersion {
		resp.Diagnostics.AddAttributeWarning(
			path.Root("api_version"),
			"API version differs from the provider's built-against version",
			fmt.Sprintf("This provider release was generated and tested against API version %s; you pinned %s. "+
				"Newer response shapes may not be fully mapped.", skycloak.DefaultAPIVersion, apiVersion),
		)
	}

	// A missing key is not an error here: a configuration may use one product
	// only. Resources fail at plan when the key they need is missing.
	userAgent := skycloak.WithUserAgent("terraform-provider-skycloak/" + p.version)
	clients := &providerClients{}
	if apiKey != "" {
		clients.keycloak = skycloak.New(endpoint, apiKey, apiVersion, userAgent)
	}
	if platformAPIKey != "" {
		clients.platform = skycloak.NewPlatform(platformEndpoint, platformAPIKey, userAgent)
	}
	resp.ResourceData = clients
	resp.DataSourceData = clients
	resp.ActionData = clients
}

func (p *skycloakProvider) Resources(_ context.Context) []func() resource.Resource {
	return gateAll(keycloakProduct, gateResource, []func() resource.Resource{
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
		NewCustomThemeResource,
		NewPasswordBlacklistFileResource,
		NewCustomExtensionResource,
		NewClusterSecurityResource,
		NewRealmRoleResource,
		NewRealmGroupResource,
		NewRealmUserResource,
		NewRealmUserRoleAssignmentResource,
		NewRealmGroupMembershipResource,
		NewApplicationRoleAssignmentResource,
		NewClusterMaintenanceWindowResource,
		NewClusterFeaturesResource,
		NewClusterPrivateLinkResource,
		NewClusterPublicAccessResource,
		NewCAPTCHADomainResource,
		NewSIEMDestinationResource,
		NewWebhookSubscriptionResource,
		NewRealmExportResource,
		NewRealmImportResource,
		NewOpenFGAInstanceResource,
	})
}

func (p *skycloakProvider) Actions(_ context.Context) []func() action.Action {
	return gateAll(keycloakProduct, gateAction, []func() action.Action{
		NewTestSMTPAction,
		NewTestIdentityProviderAction,
		NewCancelClusterUpgradeAction,
		NewTestSIEMDestinationAction,
		NewTestWebhookSubscriptionAction,
	})
}

func (p *skycloakProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return gateAll(keycloakProduct, gateDataSource, []func() datasource.DataSource{
		NewClusterDataSource,
		NewClusterLocationsDataSource,
		NewClusterTypesDataSource,
		NewClusterFeaturesDataSource,
		NewThemesDataSource,
		NewExtensionsDataSource,
		NewRealmRolesDataSource,
		NewRealmGroupsDataSource,
		NewRealmUsersDataSource,
		NewApplicationRolesDataSource,
		NewApplicationSessionsDataSource,
		NewClusterVersionsDataSource,
		NewIdentityProviderTemplatesDataSource,
		NewDomainRoutesDataSource,
		NewClusterUpgradesDataSource,
		NewOIDCDiscoveryDataSource,
		NewClusterInsightsDataSource,
		NewClusterCredentialsDataSource,
		NewRealmGroupMembersDataSource,
		NewClusterUpgradePathDataSource,
		NewClusterLogsDataSource,
		NewClusterSecurityLogsDataSource,
		NewClusterEventsDataSource,
		NewClusterEventsExportDataSource,
		NewSIEMDestinationDataSource,
		NewSIEMDestinationsDataSource,
		NewWebhookSubscriptionsDataSource,
		NewWebhookEventTypesDataSource,
		NewOpenFGAInstanceDataSource,
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
