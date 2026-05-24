package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// configuredClient extracts the client from datasource ConfigureRequest data.
func configuredClient(req datasource.ConfigureRequest, diags interface {
	AddError(string, string)
}) *skycloak.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*skycloak.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *skycloak.Client, got %T", req.ProviderData))
		return nil
	}
	return client
}

// ---- skycloak_cluster_locations ----

var (
	_ datasource.DataSource              = (*clusterLocationsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterLocationsDataSource)(nil)
)

type clusterLocationsDataSource struct{ client *skycloak.Client }

// NewClusterLocationsDataSource returns the skycloak_cluster_locations data source.
func NewClusterLocationsDataSource() datasource.DataSource { return &clusterLocationsDataSource{} }

type clusterLocationModel struct {
	Location  types.String `tfsdk:"location"`
	Name      types.String `tfsdk:"name"`
	Available types.Bool   `tfsdk:"available"`
}

type clusterLocationsModel struct {
	Locations []clusterLocationModel `tfsdk:"locations"`
}

func (d *clusterLocationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_locations"
}

func (d *clusterLocationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Supported Skycloak deployment regions.",
		Attributes: map[string]schema.Attribute{
			"locations": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"location":  schema.StringAttribute{Computed: true, MarkdownDescription: "Region code (`us`, `ca`, `eu`, `au`)."},
						"name":      schema.StringAttribute{Computed: true, MarkdownDescription: "Human-readable region name."},
						"available": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the region is available to this workspace."},
					},
				},
			},
		},
	}
}

func (d *clusterLocationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterLocationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	locations, err := d.client.ListClusterLocations(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list cluster locations", err.Error())
		return
	}
	var state clusterLocationsModel
	for _, l := range locations {
		state.Locations = append(state.Locations, clusterLocationModel{
			Location: types.StringValue(l.Location), Name: types.StringValue(l.Name), Available: types.BoolValue(l.Available),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_cluster_types ----

var (
	_ datasource.DataSource              = (*clusterTypesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterTypesDataSource)(nil)
)

type clusterTypesDataSource struct{ client *skycloak.Client }

// NewClusterTypesDataSource returns the skycloak_cluster_types data source.
func NewClusterTypesDataSource() datasource.DataSource { return &clusterTypesDataSource{} }

type clusterTypeModel struct {
	Type      types.String `tfsdk:"type"`
	Name      types.String `tfsdk:"name"`
	Available types.Bool   `tfsdk:"available"`
}

type clusterTypesModel struct {
	Types []clusterTypeModel `tfsdk:"types"`
}

func (d *clusterTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_types"
}

func (d *clusterTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Supported Skycloak cluster types.",
		Attributes: map[string]schema.Attribute{
			"types": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type":      schema.StringAttribute{Computed: true, MarkdownDescription: "Type identifier (`keycloak`, `tidecloak`)."},
						"name":      schema.StringAttribute{Computed: true, MarkdownDescription: "Human-readable type name."},
						"available": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the workspace can provision this type."},
					},
				},
			},
		},
	}
}

func (d *clusterTypesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterTypesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	typesList, err := d.client.ListClusterTypes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list cluster types", err.Error())
		return
	}
	var state clusterTypesModel
	for _, t := range typesList {
		state.Types = append(state.Types, clusterTypeModel{
			Type: types.StringValue(t.Type), Name: types.StringValue(t.Name), Available: types.BoolValue(t.Available),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_cluster_features ----

var (
	_ datasource.DataSource              = (*clusterFeaturesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterFeaturesDataSource)(nil)
)

type clusterFeaturesDataSource struct{ client *skycloak.Client }

// NewClusterFeaturesDataSource returns the skycloak_cluster_features data source.
func NewClusterFeaturesDataSource() datasource.DataSource { return &clusterFeaturesDataSource{} }

type clusterFeatureModel struct {
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"display_name"`
	Description types.String `tfsdk:"description"`
	Preview     types.Bool   `tfsdk:"preview"`
	MinVersion  types.String `tfsdk:"min_version"`
	MaxVersion  types.String `tfsdk:"max_version"`
}

type clusterFeaturesModel struct {
	Features []clusterFeatureModel `tfsdk:"features"`
}

func (d *clusterFeaturesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_features"
}

func (d *clusterFeaturesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Keycloak feature flags available to tenant clusters.",
		Attributes: map[string]schema.Attribute{
			"features": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Feature flag identifier."},
						"display_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Human-readable feature name."},
						"description":  schema.StringAttribute{Computed: true, MarkdownDescription: "What the feature does."},
						"preview":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the feature is in preview."},
						"min_version":  schema.StringAttribute{Computed: true, MarkdownDescription: "Minimum cluster version required."},
						"max_version":  schema.StringAttribute{Computed: true, MarkdownDescription: "Maximum cluster version supported."},
					},
				},
			},
		},
	}
}

func (d *clusterFeaturesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterFeaturesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	features, err := d.client.ListClusterFeatures(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list cluster features", err.Error())
		return
	}
	var state clusterFeaturesModel
	for _, f := range features {
		state.Features = append(state.Features, clusterFeatureModel{
			Name:        types.StringValue(f.Name),
			DisplayName: types.StringValue(f.DisplayName),
			Description: stringPtrToValue(f.Description),
			Preview:     types.BoolValue(f.Preview),
			MinVersion:  stringPtrToValue(f.MinVersion),
			MaxVersion:  stringPtrToValue(f.MaxVersion),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func stringPtrToValue(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}
