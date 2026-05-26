package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// ---- skycloak_cluster_insights ----

var (
	_ datasource.DataSource              = (*clusterInsightsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterInsightsDataSource)(nil)
)

type clusterInsightsDataSource struct{ client *skycloak.Client }

// NewClusterInsightsDataSource returns the skycloak_cluster_insights data source.
func NewClusterInsightsDataSource() datasource.DataSource { return &clusterInsightsDataSource{} }

type clusterInsightsModel struct {
	ClusterID types.String `tfsdk:"cluster_id"`
	Type      types.String `tfsdk:"type"`
	JSON      types.String `tfsdk:"json"`
}

func (d *clusterInsightsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_insights"
}

func (d *clusterInsightsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Cluster analytics returned as a raw JSON document. Decode with `jsondecode()`.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"type":       schema.StringAttribute{Required: true, MarkdownDescription: "Insight type: `overview`, `authentication`, `events`, `performance`, or `security`."},
			"json":       schema.StringAttribute{Computed: true, MarkdownDescription: "The analytics document as a JSON string."},
		},
	}
}

func (d *clusterInsightsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterInsightsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterInsightsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	raw, err := d.client.ClusterInsights(ctx, cfg.ClusterID.ValueString(), cfg.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cluster insights", err.Error())
		return
	}
	cfg.JSON = types.StringValue(string(raw))
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ---- skycloak_cluster_credentials ----

var (
	_ datasource.DataSource              = (*clusterCredentialsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterCredentialsDataSource)(nil)
)

type clusterCredentialsDataSource struct{ client *skycloak.Client }

// NewClusterCredentialsDataSource returns the skycloak_cluster_credentials data source.
func NewClusterCredentialsDataSource() datasource.DataSource { return &clusterCredentialsDataSource{} }

type clusterCredentialsModel struct {
	ClusterID     types.String `tfsdk:"cluster_id"`
	AdminUsername types.String `tfsdk:"admin_username"`
	AdminPassword types.String `tfsdk:"admin_password"`
}

func (d *clusterCredentialsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_credentials"
}

func (d *clusterCredentialsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A cluster's Keycloak admin credentials. Treat the password as sensitive.",
		Attributes: map[string]schema.Attribute{
			"cluster_id":     schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"admin_username": schema.StringAttribute{Computed: true, MarkdownDescription: "Keycloak admin console username."},
			"admin_password": schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Keycloak admin console password."},
		},
	}
}

func (d *clusterCredentialsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterCredentialsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterCredentialsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	creds, err := d.client.GetClusterCredentials(ctx, cfg.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cluster credentials", err.Error())
		return
	}
	cfg.AdminUsername = types.StringValue(creds.AdminUsername)
	cfg.AdminPassword = types.StringValue(creds.AdminPassword)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ---- skycloak_realm_group_members ----

var (
	_ datasource.DataSource              = (*realmGroupMembersDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*realmGroupMembersDataSource)(nil)
)

type realmGroupMembersDataSource struct{ client *skycloak.Client }

// NewRealmGroupMembersDataSource returns the skycloak_realm_group_members data source.
func NewRealmGroupMembersDataSource() datasource.DataSource { return &realmGroupMembersDataSource{} }

type realmGroupMembersModel struct {
	ClusterID types.String          `tfsdk:"cluster_id"`
	RealmName types.String          `tfsdk:"realm_name"`
	GroupID   types.String          `tfsdk:"group_id"`
	Members   []realmUserEntryModel `tfsdk:"members"`
}

func (d *realmGroupMembersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_group_members"
}

func (d *realmGroupMembersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Users that belong to a realm group.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name."},
			"group_id":   schema.StringAttribute{Required: true, MarkdownDescription: "Group ID."},
			"members": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "User ID."},
						"username":       schema.StringAttribute{Computed: true, MarkdownDescription: "Username."},
						"email":          schema.StringAttribute{Computed: true, MarkdownDescription: "Email address."},
						"first_name":     schema.StringAttribute{Computed: true, MarkdownDescription: "First name."},
						"last_name":      schema.StringAttribute{Computed: true, MarkdownDescription: "Last name."},
						"enabled":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the user can sign in."},
						"email_verified": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the email is verified."},
					},
				},
			},
		},
	}
}

func (d *realmGroupMembersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *realmGroupMembersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg realmGroupMembersModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	users, err := d.client.ListRealmGroupMembers(ctx, cfg.ClusterID.ValueString(), cfg.RealmName.ValueString(), cfg.GroupID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list group members", err.Error())
		return
	}
	state := realmGroupMembersModel{ClusterID: cfg.ClusterID, RealmName: cfg.RealmName, GroupID: cfg.GroupID}
	for _, u := range users {
		state.Members = append(state.Members, realmUserEntryModel{
			ID: types.StringValue(u.ID), Username: types.StringValue(u.Username), Email: types.StringValue(u.Email),
			FirstName: optionalString(u.FirstName), LastName: optionalString(u.LastName),
			Enabled: types.BoolValue(u.Enabled), EmailVerified: types.BoolValue(u.EmailVerified),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_cluster_upgrade_path ----

var (
	_ datasource.DataSource              = (*clusterUpgradePathDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterUpgradePathDataSource)(nil)
)

type clusterUpgradePathDataSource struct{ client *skycloak.Client }

// NewClusterUpgradePathDataSource returns the skycloak_cluster_upgrade_path data source.
func NewClusterUpgradePathDataSource() datasource.DataSource { return &clusterUpgradePathDataSource{} }

type upgradePathStepModel struct {
	Version  types.String `tfsdk:"version"`
	Required types.Bool   `tfsdk:"required"`
}

type clusterUpgradePathModel struct {
	ClusterID types.String           `tfsdk:"cluster_id"`
	Steps     []upgradePathStepModel `tfsdk:"steps"`
}

func (d *clusterUpgradePathDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_upgrade_path"
}

func (d *clusterUpgradePathDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Recommended version-upgrade path for a cluster, ordered from current to target.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"steps": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"version":  schema.StringAttribute{Computed: true, MarkdownDescription: "Version at this step."},
						"required": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this version must be visited as an intermediate step."},
					},
				},
			},
		},
	}
}

func (d *clusterUpgradePathDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterUpgradePathDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterUpgradePathModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	steps, err := d.client.GetClusterUpgradePath(ctx, cfg.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read upgrade path", err.Error())
		return
	}
	state := clusterUpgradePathModel{ClusterID: cfg.ClusterID}
	for _, s := range steps {
		state.Steps = append(state.Steps, upgradePathStepModel{Version: types.StringValue(s.Version), Required: types.BoolValue(s.Required)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
