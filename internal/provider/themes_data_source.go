package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ datasource.DataSource              = (*themesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*themesDataSource)(nil)
)

type themesDataSource struct{ client *skycloak.Client }

// NewThemesDataSource returns the skycloak_themes data source.
func NewThemesDataSource() datasource.DataSource { return &themesDataSource{} }

type themeModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Version     types.String `tfsdk:"version"`
	Status      types.String `tfsdk:"status"`
	ThemeTypes  types.List   `tfsdk:"theme_types"`
	FileSize    types.Int64  `tfsdk:"file_size"`
	DeployedAt  types.String `tfsdk:"deployed_at"`
}

type themesModel struct {
	ClusterID types.String `tfsdk:"cluster_id"`
	Themes    []themeModel `tfsdk:"themes"`
}

func (d *themesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_themes"
}

func (d *themesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Custom themes uploaded to a cluster. Use a theme's `id` in `skycloak_theme_assignment` or `skycloak_client_theme_assignment`.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID to list themes for."},
			"themes": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Theme ID."},
						"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Theme name."},
						"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Theme description."},
						"version":     schema.StringAttribute{Computed: true, MarkdownDescription: "Theme version."},
						"status":      schema.StringAttribute{Computed: true, MarkdownDescription: "Deployment status (`deployed`, `deploying`, `failed`, `undeploying`)."},
						"theme_types": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Keycloak theme types in the package (`login`, `account`, `admin`, `email`)."},
						"file_size":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the uploaded archive in bytes."},
						"deployed_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Timestamp of the last successful deployment (RFC 3339)."},
					},
				},
			},
		},
	}
}

func (d *themesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *themesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg themesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	themes, err := d.client.ListThemes(ctx, cfg.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list themes", err.Error())
		return
	}
	state := themesModel{ClusterID: cfg.ClusterID}
	for _, t := range themes {
		state.Themes = append(state.Themes, themeModel{
			ID:          types.StringValue(t.ID),
			Name:        types.StringValue(t.Name),
			Description: optionalString(t.Description),
			Version:     optionalString(t.Version),
			Status:      types.StringValue(t.Status),
			ThemeTypes:  sliceToStringList(ctx, t.ThemeTypes, &resp.Diagnostics),
			FileSize:    types.Int64Value(t.FileSize),
			DeployedAt:  optionalString(t.DeployedAt),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
