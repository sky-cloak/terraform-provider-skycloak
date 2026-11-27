package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// product names one of the two Skycloak APIs the provider talks to and the
// provider argument and environment variable that carry its key. A
// configuration may set one key, the other or both, so a missing key is only an
// error for the resources, data sources and actions that need it.
type product struct {
	name      string
	attribute string
	envVar    string
}

var (
	keycloakProduct = product{name: "managed Keycloak", attribute: "api_key", envVar: "SKYCLOAK_API_KEY"}
	platformProduct = product{name: "Platform", attribute: "platform_api_key", envVar: "SKYCLOAK_PLATFORM_API_KEY"}
)

func (p product) missingKeyError(typeName string) (summary, detail string) {
	return fmt.Sprintf("Missing %s API key", p.name),
		fmt.Sprintf("%s needs a %s API key. Set the %s provider argument or the %s environment variable.",
			typeName, p.name, p.attribute, p.envVar)
}

// providerClients is what the provider hands to every resource, data source
// and action. A nil client means its key was not configured.
type providerClients struct {
	keycloak *skycloak.Client
	platform *skycloak.PlatformClient
}

// forProduct returns the client a product's resources use, or nil when that
// product's key is missing.
func (c *providerClients) forProduct(p product) any {
	switch p {
	case keycloakProduct:
		if c.keycloak != nil {
			return c.keycloak
		}
	case platformProduct:
		if c.platform != nil {
			return c.platform
		}
	}
	return nil
}

// resolve turns the provider data into the one client the wrapped object
// expects. Data that is not providerClients is passed through untouched.
func resolve(data any, p product, typeName string) (client any, summary, detail string) {
	clients, ok := data.(*providerClients)
	if !ok {
		return data, "", ""
	}
	client = clients.forProduct(p)
	if client == nil {
		summary, detail = p.missingKeyError(typeName)
	}
	return client, summary, detail
}

// gateResource wraps a resource so it receives the client for its product, and
// so that planning it fails when that product's key is missing.
func gateResource(p product, newResource func() resource.Resource) func() resource.Resource {
	return func() resource.Resource { return &gatedResource{Resource: newResource(), product: p} }
}

type gatedResource struct {
	resource.Resource
	product product
}

var (
	_ resource.ResourceWithConfigure      = (*gatedResource)(nil)
	_ resource.ResourceWithImportState    = (*gatedResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*gatedResource)(nil)
	_ resource.ResourceWithValidateConfig = (*gatedResource)(nil)
)

func (g *gatedResource) typeName(ctx context.Context) string {
	var resp resource.MetadataResponse
	g.Resource.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "skycloak"}, &resp)
	return resp.TypeName
}

func (g *gatedResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, summary, detail := resolve(req.ProviderData, g.product, g.typeName(ctx))
	if client == nil {
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	if inner, ok := g.Resource.(resource.ResourceWithConfigure); ok {
		req.ProviderData = client
		inner.Configure(ctx, req, resp)
	}
}

func (g *gatedResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if inner, ok := g.Resource.(resource.ResourceWithImportState); ok {
		inner.ImportState(ctx, req, resp)
		return
	}
	resp.Diagnostics.AddError("Resource Import Not Implemented",
		fmt.Sprintf("%s does not support import.", g.typeName(ctx)))
}

func (g *gatedResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if inner, ok := g.Resource.(resource.ResourceWithModifyPlan); ok {
		inner.ModifyPlan(ctx, req, resp)
	}
}

func (g *gatedResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	if inner, ok := g.Resource.(resource.ResourceWithValidateConfig); ok {
		inner.ValidateConfig(ctx, req, resp)
	}
}

// gateDataSource is gateResource for data sources.
func gateDataSource(p product, newDataSource func() datasource.DataSource) func() datasource.DataSource {
	return func() datasource.DataSource { return &gatedDataSource{DataSource: newDataSource(), product: p} }
}

type gatedDataSource struct {
	datasource.DataSource
	product product
}

var _ datasource.DataSourceWithConfigure = (*gatedDataSource)(nil)

func (g *gatedDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var meta datasource.MetadataResponse
	g.DataSource.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "skycloak"}, &meta)
	client, summary, detail := resolve(req.ProviderData, g.product, meta.TypeName)
	if client == nil {
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	if inner, ok := g.DataSource.(datasource.DataSourceWithConfigure); ok {
		req.ProviderData = client
		inner.Configure(ctx, req, resp)
	}
}

// gateAction is gateResource for actions.
func gateAction(p product, newAction func() action.Action) func() action.Action {
	return func() action.Action { return &gatedAction{Action: newAction(), product: p} }
}

type gatedAction struct {
	action.Action
	product product
}

var _ action.ActionWithConfigure = (*gatedAction)(nil)

func (g *gatedAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	var meta action.MetadataResponse
	g.Action.Metadata(ctx, action.MetadataRequest{ProviderTypeName: "skycloak"}, &meta)
	client, summary, detail := resolve(req.ProviderData, g.product, meta.TypeName)
	if client == nil {
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	if inner, ok := g.Action.(action.ActionWithConfigure); ok {
		req.ProviderData = client
		inner.Configure(ctx, req, resp)
	}
}

// gateAll wraps every constructor in a list with the gate for one product.
func gateAll[T any](p product, gate func(product, func() T) func() T, all []func() T) []func() T {
	out := make([]func() T, len(all))
	for i, f := range all {
		out[i] = gate(p, f)
	}
	return out
}
