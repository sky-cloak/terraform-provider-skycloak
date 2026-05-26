package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// ---- skycloak_cluster_versions ----

var (
	_ datasource.DataSource              = (*clusterVersionsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterVersionsDataSource)(nil)
)

type clusterVersionsDataSource struct{ client *skycloak.Client }

// NewClusterVersionsDataSource returns the skycloak_cluster_versions data source.
func NewClusterVersionsDataSource() datasource.DataSource { return &clusterVersionsDataSource{} }

type clusterVersionsModel struct {
	Type     types.String `tfsdk:"type"`
	Versions types.List   `tfsdk:"versions"`
}

func (d *clusterVersionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_versions"
}

func (d *clusterVersionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Keycloak versions available for a cluster type.",
		Attributes: map[string]schema.Attribute{
			"type":     schema.StringAttribute{Required: true, MarkdownDescription: "Cluster type (`keycloak`, `tidecloak`)."},
			"versions": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Supported versions, newest first."},
		},
	}
}

func (d *clusterVersionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterVersionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterVersionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	versions, err := d.client.ClusterTypeVersions(ctx, cfg.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list cluster versions", err.Error())
		return
	}
	cfg.Versions = sliceToStringList(ctx, versions, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ---- skycloak_identity_provider_templates ----

var (
	_ datasource.DataSource              = (*identityProviderTemplatesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*identityProviderTemplatesDataSource)(nil)
)

type identityProviderTemplatesDataSource struct{ client *skycloak.Client }

// NewIdentityProviderTemplatesDataSource returns the skycloak_identity_provider_templates data source.
func NewIdentityProviderTemplatesDataSource() datasource.DataSource {
	return &identityProviderTemplatesDataSource{}
}

type providerTemplateEntryModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
}

type identityProviderTemplatesModel struct {
	Templates []providerTemplateEntryModel `tfsdk:"templates"`
}

func (d *identityProviderTemplatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_identity_provider_templates"
}

func (d *identityProviderTemplatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Pre-configured identity-provider templates. Use a template `id` as `template_id` when creating an identity provider.",
		Attributes: map[string]schema.Attribute{
			"templates": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Template ID."},
						"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Template name."},
						"description": schema.StringAttribute{Computed: true, MarkdownDescription: "What the template configures."},
						"type":        schema.StringAttribute{Computed: true, MarkdownDescription: "Identity provider protocol."},
					},
				},
			},
		},
	}
}

func (d *identityProviderTemplatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *identityProviderTemplatesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	templates, err := d.client.ListIdentityProviderTemplates(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list identity provider templates", err.Error())
		return
	}
	var state identityProviderTemplatesModel
	for _, t := range templates {
		state.Templates = append(state.Templates, providerTemplateEntryModel{
			ID: types.StringValue(t.ID), Name: types.StringValue(t.Name), Description: optionalString(t.Description), Type: types.StringValue(t.Type),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_domain_routes ----

var (
	_ datasource.DataSource              = (*domainRoutesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*domainRoutesDataSource)(nil)
)

type domainRoutesDataSource struct{ client *skycloak.Client }

// NewDomainRoutesDataSource returns the skycloak_domain_routes data source.
func NewDomainRoutesDataSource() datasource.DataSource { return &domainRoutesDataSource{} }

type domainRouteEntryModel struct {
	ID               types.String `tfsdk:"id"`
	Realm            types.String `tfsdk:"realm"`
	AllowAdminAccess types.Bool   `tfsdk:"allow_admin_access"`
	HideRealmPath    types.Bool   `tfsdk:"hide_realm_path"`
}

type domainRoutesModel struct {
	ClusterID types.String            `tfsdk:"cluster_id"`
	DomainID  types.String            `tfsdk:"domain_id"`
	Routes    []domainRouteEntryModel `tfsdk:"routes"`
}

func (d *domainRoutesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain_routes"
}

func (d *domainRoutesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Realm routes configured on a custom domain.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"domain_id":  schema.StringAttribute{Required: true, MarkdownDescription: "Domain ID."},
			"routes": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Route ID."},
						"realm":              schema.StringAttribute{Computed: true, MarkdownDescription: "Realm mapped onto the domain."},
						"allow_admin_access": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the admin console is reachable."},
						"hide_realm_path":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the realm path is hidden."},
					},
				},
			},
		},
	}
}

func (d *domainRoutesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *domainRoutesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg domainRoutesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	routes, err := d.client.ListDomainRoutes(ctx, cfg.ClusterID.ValueString(), cfg.DomainID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list domain routes", err.Error())
		return
	}
	state := domainRoutesModel{ClusterID: cfg.ClusterID, DomainID: cfg.DomainID}
	for _, r := range routes {
		state.Routes = append(state.Routes, domainRouteEntryModel{
			ID: types.StringValue(r.ID), Realm: types.StringValue(r.Realm),
			AllowAdminAccess: types.BoolValue(r.AllowAdminAccess), HideRealmPath: types.BoolValue(r.HideRealmPath),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_cluster_builds ----

var (
	_ datasource.DataSource              = (*clusterBuildsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterBuildsDataSource)(nil)
)

type clusterBuildsDataSource struct{ client *skycloak.Client }

// NewClusterBuildsDataSource returns the skycloak_cluster_builds data source.
func NewClusterBuildsDataSource() datasource.DataSource { return &clusterBuildsDataSource{} }

type clusterBuildEntryModel struct {
	ID          types.String `tfsdk:"id"`
	Status      types.String `tfsdk:"status"`
	Phase       types.String `tfsdk:"phase"`
	Progress    types.Int64  `tfsdk:"progress"`
	Error       types.String `tfsdk:"error"`
	StartedAt   types.String `tfsdk:"started_at"`
	CompletedAt types.String `tfsdk:"completed_at"`
}

type clusterBuildsModel struct {
	ClusterID types.String             `tfsdk:"cluster_id"`
	Builds    []clusterBuildEntryModel `tfsdk:"builds"`
}

func (d *clusterBuildsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_builds"
}

func (d *clusterBuildsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Image build history for a cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"builds": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Build ID."},
						"status":       schema.StringAttribute{Computed: true, MarkdownDescription: "Build status."},
						"phase":        schema.StringAttribute{Computed: true, MarkdownDescription: "Current build phase."},
						"progress":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Percentage complete."},
						"error":        schema.StringAttribute{Computed: true, MarkdownDescription: "Failure detail, when failed."},
						"started_at":   schema.StringAttribute{Computed: true, MarkdownDescription: "Start time (RFC 3339)."},
						"completed_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Completion time (RFC 3339)."},
					},
				},
			},
		},
	}
}

func (d *clusterBuildsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterBuildsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterBuildsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	builds, err := d.client.ListClusterBuilds(ctx, cfg.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list cluster builds", err.Error())
		return
	}
	state := clusterBuildsModel{ClusterID: cfg.ClusterID}
	for _, b := range builds {
		state.Builds = append(state.Builds, clusterBuildEntryModel{
			ID: types.StringValue(b.ID), Status: types.StringValue(b.Status), Phase: types.StringValue(b.Phase),
			Progress: types.Int64Value(b.Progress), Error: optionalString(b.Error),
			StartedAt: types.StringValue(b.StartedAt), CompletedAt: optionalString(b.CompletedAt),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_cluster_upgrades ----

var (
	_ datasource.DataSource              = (*clusterUpgradesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterUpgradesDataSource)(nil)
)

type clusterUpgradesDataSource struct{ client *skycloak.Client }

// NewClusterUpgradesDataSource returns the skycloak_cluster_upgrades data source.
func NewClusterUpgradesDataSource() datasource.DataSource { return &clusterUpgradesDataSource{} }

type clusterUpgradeEntryModel struct {
	ID          types.String `tfsdk:"id"`
	FromVersion types.String `tfsdk:"from_version"`
	ToVersion   types.String `tfsdk:"to_version"`
	Phase       types.String `tfsdk:"phase"`
	StartedAt   types.String `tfsdk:"started_at"`
	CompletedAt types.String `tfsdk:"completed_at"`
}

type clusterUpgradesModel struct {
	ClusterID types.String               `tfsdk:"cluster_id"`
	Upgrades  []clusterUpgradeEntryModel `tfsdk:"upgrades"`
}

func (d *clusterUpgradesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_upgrades"
}

func (d *clusterUpgradesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Version-upgrade history for a cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"upgrades": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Upgrade ID."},
						"from_version": schema.StringAttribute{Computed: true, MarkdownDescription: "Version before the upgrade."},
						"to_version":   schema.StringAttribute{Computed: true, MarkdownDescription: "Target version."},
						"phase":        schema.StringAttribute{Computed: true, MarkdownDescription: "Upgrade phase."},
						"started_at":   schema.StringAttribute{Computed: true, MarkdownDescription: "Start time (RFC 3339)."},
						"completed_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Completion time (RFC 3339)."},
					},
				},
			},
		},
	}
}

func (d *clusterUpgradesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterUpgradesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterUpgradesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	upgrades, err := d.client.ListClusterUpgrades(ctx, cfg.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list cluster upgrades", err.Error())
		return
	}
	state := clusterUpgradesModel{ClusterID: cfg.ClusterID}
	for _, u := range upgrades {
		state.Upgrades = append(state.Upgrades, clusterUpgradeEntryModel{
			ID: types.StringValue(u.ID), FromVersion: types.StringValue(u.FromVersion), ToVersion: types.StringValue(u.ToVersion),
			Phase: types.StringValue(u.Phase), StartedAt: types.StringValue(u.StartedAt), CompletedAt: optionalString(u.CompletedAt),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
