package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ datasource.DataSource              = (*extensionsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*extensionsDataSource)(nil)
)

type extensionsDataSource struct{ client *skycloak.Client }

// NewExtensionsDataSource returns the skycloak_extensions data source.
func NewExtensionsDataSource() datasource.DataSource { return &extensionsDataSource{} }

type extensionModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	Source           types.String `tfsdk:"source"`
	KeycloakVersions types.List   `tfsdk:"keycloak_versions"`
	DocumentationURL types.String `tfsdk:"documentation_url"`
	RepositoryURL    types.String `tfsdk:"repository_url"`
}

type extensionsModel struct {
	Extensions []extensionModel `tfsdk:"extensions"`
}

func (d *extensionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_extensions"
}

func (d *extensionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Extension catalog available to the workspace. Use an extension's `id` in `skycloak_cluster_extension`.",
		Attributes: map[string]schema.Attribute{
			"extensions": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                schema.StringAttribute{Computed: true, MarkdownDescription: "Extension ID."},
						"name":              schema.StringAttribute{Computed: true, MarkdownDescription: "Extension name."},
						"description":       schema.StringAttribute{Computed: true, MarkdownDescription: "What the extension does."},
						"source":            schema.StringAttribute{Computed: true, MarkdownDescription: "Source (`marketplace`, `custom`)."},
						"keycloak_versions": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Supported Keycloak major versions."},
						"documentation_url": schema.StringAttribute{Computed: true, MarkdownDescription: "Documentation URL."},
						"repository_url":    schema.StringAttribute{Computed: true, MarkdownDescription: "Source repository URL."},
					},
				},
			},
		},
	}
}

func (d *extensionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *extensionsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	exts, err := d.client.ListExtensions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list extensions", err.Error())
		return
	}
	var state extensionsModel
	for _, e := range exts {
		state.Extensions = append(state.Extensions, extensionModel{
			ID:               types.StringValue(e.ID),
			Name:             types.StringValue(e.Name),
			Description:      optionalString(e.Description),
			Source:           types.StringValue(e.Source),
			KeycloakVersions: sliceToStringList(ctx, e.KeycloakVersions, &resp.Diagnostics),
			DocumentationURL: optionalString(e.DocumentationURL),
			RepositoryURL:    optionalString(e.RepositoryURL),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
