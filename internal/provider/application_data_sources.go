package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// ---- skycloak_application_roles ----

var (
	_ datasource.DataSource              = (*applicationRolesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*applicationRolesDataSource)(nil)
)

type applicationRolesDataSource struct{ client *skycloak.Client }

// NewApplicationRolesDataSource returns the skycloak_application_roles data source.
func NewApplicationRolesDataSource() datasource.DataSource { return &applicationRolesDataSource{} }

type applicationRoleEntryModel struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Composite   types.Bool   `tfsdk:"composite"`
	ClientRole  types.Bool   `tfsdk:"client_role"`
}

type applicationRolesModel struct {
	ClusterID types.String                `tfsdk:"cluster_id"`
	RealmName types.String                `tfsdk:"realm_name"`
	ClientID  types.String                `tfsdk:"client_id"`
	Roles     []applicationRoleEntryModel `tfsdk:"roles"`
}

func (d *applicationRolesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_roles"
}

func (d *applicationRolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Roles assigned to an application's service account.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name."},
			"client_id":  schema.StringAttribute{Required: true, MarkdownDescription: "Application client ID."},
			"roles": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Role name."},
						"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Role description."},
						"composite":   schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the role is composite."},
						"client_role": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the role is a client role."},
					},
				},
			},
		},
	}
}

func (d *applicationRolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *applicationRolesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg applicationRolesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	roles, err := d.client.ListApplicationRoles(ctx, cfg.ClusterID.ValueString(), cfg.RealmName.ValueString(), cfg.ClientID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list application roles", err.Error())
		return
	}
	state := applicationRolesModel{ClusterID: cfg.ClusterID, RealmName: cfg.RealmName, ClientID: cfg.ClientID}
	for _, r := range roles {
		state.Roles = append(state.Roles, applicationRoleEntryModel{
			Name: types.StringValue(r.Name), Description: optionalString(r.Description),
			Composite: types.BoolValue(r.Composite), ClientRole: types.BoolValue(r.ClientRole),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_application_sessions ----

var (
	_ datasource.DataSource              = (*applicationSessionsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*applicationSessionsDataSource)(nil)
)

type applicationSessionsDataSource struct{ client *skycloak.Client }

// NewApplicationSessionsDataSource returns the skycloak_application_sessions data source.
func NewApplicationSessionsDataSource() datasource.DataSource {
	return &applicationSessionsDataSource{}
}

type applicationSessionEntryModel struct {
	ID           types.String `tfsdk:"id"`
	UserID       types.String `tfsdk:"user_id"`
	Username     types.String `tfsdk:"username"`
	Email        types.String `tfsdk:"email"`
	IPAddress    types.String `tfsdk:"ip_address"`
	StartedAt    types.String `tfsdk:"started_at"`
	LastAccessAt types.String `tfsdk:"last_access_at"`
}

type applicationSessionsModel struct {
	ClusterID types.String                   `tfsdk:"cluster_id"`
	RealmName types.String                   `tfsdk:"realm_name"`
	ClientID  types.String                   `tfsdk:"client_id"`
	Sessions  []applicationSessionEntryModel `tfsdk:"sessions"`
}

func (d *applicationSessionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_sessions"
}

func (d *applicationSessionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Active user sessions for an application.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name."},
			"client_id":  schema.StringAttribute{Required: true, MarkdownDescription: "Application client ID."},
			"sessions": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Session ID."},
						"user_id":        schema.StringAttribute{Computed: true, MarkdownDescription: "User ID."},
						"username":       schema.StringAttribute{Computed: true, MarkdownDescription: "Username."},
						"email":          schema.StringAttribute{Computed: true, MarkdownDescription: "User email."},
						"ip_address":     schema.StringAttribute{Computed: true, MarkdownDescription: "Originating IP address."},
						"started_at":     schema.StringAttribute{Computed: true, MarkdownDescription: "Session start (RFC 3339)."},
						"last_access_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Last activity (RFC 3339)."},
					},
				},
			},
		},
	}
}

func (d *applicationSessionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *applicationSessionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg applicationSessionsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sessions, err := d.client.ListApplicationSessions(ctx, cfg.ClusterID.ValueString(), cfg.RealmName.ValueString(), cfg.ClientID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list application sessions", err.Error())
		return
	}
	state := applicationSessionsModel{ClusterID: cfg.ClusterID, RealmName: cfg.RealmName, ClientID: cfg.ClientID}
	for _, s := range sessions {
		state.Sessions = append(state.Sessions, applicationSessionEntryModel{
			ID: types.StringValue(s.ID), UserID: types.StringValue(s.UserID), Username: types.StringValue(s.Username),
			Email: optionalString(s.Email), IPAddress: optionalString(s.IPAddress),
			StartedAt: types.StringValue(s.StartedAt), LastAccessAt: types.StringValue(s.LastAccessAt),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
