package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// ---- skycloak_realm_roles ----

var (
	_ datasource.DataSource              = (*realmRolesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*realmRolesDataSource)(nil)
)

type realmRolesDataSource struct{ client *skycloak.Client }

// NewRealmRolesDataSource returns the skycloak_realm_roles data source.
func NewRealmRolesDataSource() datasource.DataSource { return &realmRolesDataSource{} }

type realmRoleEntryModel struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Composite   types.Bool   `tfsdk:"composite"`
	ClientRole  types.Bool   `tfsdk:"client_role"`
}

type realmRolesModel struct {
	ClusterID types.String          `tfsdk:"cluster_id"`
	RealmName types.String          `tfsdk:"realm_name"`
	Roles     []realmRoleEntryModel `tfsdk:"roles"`
}

func (d *realmRolesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_roles"
}

func (d *realmRolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Realm-scoped roles in a realm.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name."},
			"roles": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Role name."},
						"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Role description."},
						"composite":   schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the role is composite."},
						"client_role": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the role is client-scoped."},
					},
				},
			},
		},
	}
}

func (d *realmRolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *realmRolesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg realmRolesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	roles, err := d.client.ListRealmRoles(ctx, cfg.ClusterID.ValueString(), cfg.RealmName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list realm roles", err.Error())
		return
	}
	state := realmRolesModel{ClusterID: cfg.ClusterID, RealmName: cfg.RealmName}
	for _, r := range roles {
		state.Roles = append(state.Roles, realmRoleEntryModel{
			Name: types.StringValue(r.Name), Description: optionalString(r.Description),
			Composite: types.BoolValue(r.Composite), ClientRole: types.BoolValue(r.ClientRole),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_realm_groups ----

var (
	_ datasource.DataSource              = (*realmGroupsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*realmGroupsDataSource)(nil)
)

type realmGroupsDataSource struct{ client *skycloak.Client }

// NewRealmGroupsDataSource returns the skycloak_realm_groups data source.
func NewRealmGroupsDataSource() datasource.DataSource { return &realmGroupsDataSource{} }

type realmGroupEntryModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Path types.String `tfsdk:"path"`
}

type realmGroupsModel struct {
	ClusterID types.String           `tfsdk:"cluster_id"`
	RealmName types.String           `tfsdk:"realm_name"`
	Groups    []realmGroupEntryModel `tfsdk:"groups"`
}

func (d *realmGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_groups"
}

func (d *realmGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Top-level groups in a realm.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name."},
			"groups": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Group ID."},
						"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Group name."},
						"path": schema.StringAttribute{Computed: true, MarkdownDescription: "Canonical group path."},
					},
				},
			},
		},
	}
}

func (d *realmGroupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *realmGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg realmGroupsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groups, err := d.client.ListRealmGroups(ctx, cfg.ClusterID.ValueString(), cfg.RealmName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list realm groups", err.Error())
		return
	}
	state := realmGroupsModel{ClusterID: cfg.ClusterID, RealmName: cfg.RealmName}
	for _, g := range groups {
		state.Groups = append(state.Groups, realmGroupEntryModel{
			ID: types.StringValue(g.ID), Name: types.StringValue(g.Name), Path: types.StringValue(g.Path),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_realm_users ----

var (
	_ datasource.DataSource              = (*realmUsersDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*realmUsersDataSource)(nil)
)

type realmUsersDataSource struct{ client *skycloak.Client }

// NewRealmUsersDataSource returns the skycloak_realm_users data source.
func NewRealmUsersDataSource() datasource.DataSource { return &realmUsersDataSource{} }

type realmUserEntryModel struct {
	ID            types.String `tfsdk:"id"`
	Username      types.String `tfsdk:"username"`
	Email         types.String `tfsdk:"email"`
	FirstName     types.String `tfsdk:"first_name"`
	LastName      types.String `tfsdk:"last_name"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	EmailVerified types.Bool   `tfsdk:"email_verified"`
}

type realmUsersModel struct {
	ClusterID types.String          `tfsdk:"cluster_id"`
	RealmName types.String          `tfsdk:"realm_name"`
	Users     []realmUserEntryModel `tfsdk:"users"`
}

func (d *realmUsersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_users"
}

func (d *realmUsersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Users in a realm.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name."},
			"users": schema.ListNestedAttribute{
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

func (d *realmUsersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *realmUsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg realmUsersModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	users, err := d.client.ListRealmUsers(ctx, cfg.ClusterID.ValueString(), cfg.RealmName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list realm users", err.Error())
		return
	}
	state := realmUsersModel{ClusterID: cfg.ClusterID, RealmName: cfg.RealmName}
	for _, u := range users {
		state.Users = append(state.Users, realmUserEntryModel{
			ID: types.StringValue(u.ID), Username: types.StringValue(u.Username), Email: types.StringValue(u.Email),
			FirstName: optionalString(u.FirstName), LastName: optionalString(u.LastName),
			Enabled: types.BoolValue(u.Enabled), EmailVerified: types.BoolValue(u.EmailVerified),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
