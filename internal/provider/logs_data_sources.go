package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// ---- skycloak_cluster_logs ----

var (
	_ datasource.DataSource              = (*clusterLogsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterLogsDataSource)(nil)
)

type clusterLogsDataSource struct{ client *skycloak.Client }

// NewClusterLogsDataSource returns the skycloak_cluster_logs data source.
func NewClusterLogsDataSource() datasource.DataSource { return &clusterLogsDataSource{} }

type logEntryModel struct {
	Timestamp types.String `tfsdk:"timestamp"`
	Level     types.String `tfsdk:"level"`
	Category  types.String `tfsdk:"category"`
	Message   types.String `tfsdk:"message"`
	Source    types.String `tfsdk:"source"`
}

type clusterLogsModel struct {
	ClusterID types.String    `tfsdk:"cluster_id"`
	Level     types.String    `tfsdk:"level"`
	Search    types.String    `tfsdk:"search"`
	Limit     types.Int64     `tfsdk:"limit"`
	Logs      []logEntryModel `tfsdk:"logs"`
}

func (d *clusterLogsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_logs"
}

func (d *clusterLogsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Recent application logs for a cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"level":      schema.StringAttribute{Optional: true, MarkdownDescription: "Filter by log level."},
			"search":     schema.StringAttribute{Optional: true, MarkdownDescription: "Full-text search filter."},
			"limit":      schema.Int64Attribute{Optional: true, MarkdownDescription: "Maximum entries to return."},
			"logs": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"timestamp": schema.StringAttribute{Computed: true, MarkdownDescription: "Entry time (RFC 3339)."},
						"level":     schema.StringAttribute{Computed: true, MarkdownDescription: "Log level."},
						"category":  schema.StringAttribute{Computed: true, MarkdownDescription: "Logger category."},
						"message":   schema.StringAttribute{Computed: true, MarkdownDescription: "Log message."},
						"source":    schema.StringAttribute{Computed: true, MarkdownDescription: "Producing component."},
					},
				},
			},
		},
	}
}

func (d *clusterLogsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterLogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterLogsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	logs, err := d.client.ListClusterLogs(ctx, cfg.ClusterID.ValueString(), skycloak.LogQuery{
		Limit: int(cfg.Limit.ValueInt64()), Level: cfg.Level.ValueString(), Search: cfg.Search.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cluster logs", err.Error())
		return
	}
	state := clusterLogsModel{ClusterID: cfg.ClusterID, Level: cfg.Level, Search: cfg.Search, Limit: cfg.Limit}
	for _, l := range logs {
		state.Logs = append(state.Logs, logEntryModel{
			Timestamp: types.StringValue(l.Timestamp), Level: types.StringValue(l.Level), Category: types.StringValue(l.Category),
			Message: types.StringValue(l.Message), Source: optionalString(l.Source),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_cluster_security_logs ----

var (
	_ datasource.DataSource              = (*clusterSecurityLogsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterSecurityLogsDataSource)(nil)
)

type clusterSecurityLogsDataSource struct{ client *skycloak.Client }

// NewClusterSecurityLogsDataSource returns the skycloak_cluster_security_logs data source.
func NewClusterSecurityLogsDataSource() datasource.DataSource {
	return &clusterSecurityLogsDataSource{}
}

type securityLogEntryModel struct {
	Timestamp types.String `tfsdk:"timestamp"`
	Type      types.String `tfsdk:"type"`
	Action    types.String `tfsdk:"action"`
	SourceIP  types.String `tfsdk:"source_ip"`
	Country   types.String `tfsdk:"country"`
	Method    types.String `tfsdk:"method"`
	URI       types.String `tfsdk:"uri"`
	Message   types.String `tfsdk:"message"`
}

type clusterSecurityLogsModel struct {
	ClusterID types.String            `tfsdk:"cluster_id"`
	Search    types.String            `tfsdk:"search"`
	Limit     types.Int64             `tfsdk:"limit"`
	Logs      []securityLogEntryModel `tfsdk:"logs"`
}

func (d *clusterSecurityLogsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_security_logs"
}

func (d *clusterSecurityLogsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Recent edge-security logs (WAF, geo-blocking, rate limiting) for a cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"search":     schema.StringAttribute{Optional: true, MarkdownDescription: "Full-text search filter."},
			"limit":      schema.Int64Attribute{Optional: true, MarkdownDescription: "Maximum entries to return."},
			"logs": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"timestamp": schema.StringAttribute{Computed: true, MarkdownDescription: "Event time (RFC 3339)."},
						"type":      schema.StringAttribute{Computed: true, MarkdownDescription: "Event type."},
						"action":    schema.StringAttribute{Computed: true, MarkdownDescription: "Action taken."},
						"source_ip": schema.StringAttribute{Computed: true, MarkdownDescription: "Client IP."},
						"country":   schema.StringAttribute{Computed: true, MarkdownDescription: "Resolved country."},
						"method":    schema.StringAttribute{Computed: true, MarkdownDescription: "HTTP method."},
						"uri":       schema.StringAttribute{Computed: true, MarkdownDescription: "Request URI."},
						"message":   schema.StringAttribute{Computed: true, MarkdownDescription: "Event message."},
					},
				},
			},
		},
	}
}

func (d *clusterSecurityLogsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterSecurityLogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterSecurityLogsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	logs, err := d.client.ListClusterSecurityLogs(ctx, cfg.ClusterID.ValueString(), int(cfg.Limit.ValueInt64()), cfg.Search.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read security logs", err.Error())
		return
	}
	state := clusterSecurityLogsModel{ClusterID: cfg.ClusterID, Search: cfg.Search, Limit: cfg.Limit}
	for _, s := range logs {
		state.Logs = append(state.Logs, securityLogEntryModel{
			Timestamp: types.StringValue(s.Timestamp), Type: types.StringValue(s.Type), Action: types.StringValue(s.Action),
			SourceIP: optionalString(s.SourceIP), Country: optionalString(s.Country), Method: optionalString(s.Method),
			URI: optionalString(s.URI), Message: optionalString(s.Message),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- skycloak_cluster_events ----

var (
	_ datasource.DataSource              = (*clusterEventsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterEventsDataSource)(nil)
)

type clusterEventsDataSource struct{ client *skycloak.Client }

// NewClusterEventsDataSource returns the skycloak_cluster_events data source.
func NewClusterEventsDataSource() datasource.DataSource { return &clusterEventsDataSource{} }

type eventEntryModel struct {
	Timestamp types.String `tfsdk:"timestamp"`
	Category  types.String `tfsdk:"category"`
	Type      types.String `tfsdk:"type"`
	RealmName types.String `tfsdk:"realm_name"`
	ClientID  types.String `tfsdk:"client_id"`
	Username  types.String `tfsdk:"username"`
	IPAddress types.String `tfsdk:"ip_address"`
	Error     types.String `tfsdk:"error"`
}

type clusterEventsModel struct {
	ClusterID types.String      `tfsdk:"cluster_id"`
	Category  types.String      `tfsdk:"category"`
	Realm     types.String      `tfsdk:"realm"`
	Username  types.String      `tfsdk:"username"`
	Search    types.String      `tfsdk:"search"`
	Limit     types.Int64       `tfsdk:"limit"`
	Events    []eventEntryModel `tfsdk:"events"`
}

func (d *clusterEventsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_events"
}

func (d *clusterEventsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Recent Keycloak admin/user events for a cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"category":   schema.StringAttribute{Optional: true, MarkdownDescription: "Filter by event category (`user`, `admin`)."},
			"realm":      schema.StringAttribute{Optional: true, MarkdownDescription: "Filter by realm name."},
			"username":   schema.StringAttribute{Optional: true, MarkdownDescription: "Filter by username."},
			"search":     schema.StringAttribute{Optional: true, MarkdownDescription: "Full-text search filter."},
			"limit":      schema.Int64Attribute{Optional: true, MarkdownDescription: "Maximum entries to return."},
			"events": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"timestamp":  schema.StringAttribute{Computed: true, MarkdownDescription: "Event time (RFC 3339)."},
						"category":   schema.StringAttribute{Computed: true, MarkdownDescription: "Event category."},
						"type":       schema.StringAttribute{Computed: true, MarkdownDescription: "Event type."},
						"realm_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Realm name."},
						"client_id":  schema.StringAttribute{Computed: true, MarkdownDescription: "Client ID."},
						"username":   schema.StringAttribute{Computed: true, MarkdownDescription: "Username."},
						"ip_address": schema.StringAttribute{Computed: true, MarkdownDescription: "Client IP."},
						"error":      schema.StringAttribute{Computed: true, MarkdownDescription: "Error code for failed events."},
					},
				},
			},
		},
	}
}

func (d *clusterEventsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterEventsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterEventsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	events, err := d.client.ListClusterEvents(ctx, cfg.ClusterID.ValueString(), skycloak.EventQuery{
		Limit: int(cfg.Limit.ValueInt64()), Category: cfg.Category.ValueString(), Realm: cfg.Realm.ValueString(),
		Username: cfg.Username.ValueString(), Search: cfg.Search.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cluster events", err.Error())
		return
	}
	state := clusterEventsModel{ClusterID: cfg.ClusterID, Category: cfg.Category, Realm: cfg.Realm, Username: cfg.Username, Search: cfg.Search, Limit: cfg.Limit}
	for _, e := range events {
		state.Events = append(state.Events, eventEntryModel{
			Timestamp: types.StringValue(e.Timestamp), Category: types.StringValue(e.Category), Type: optionalString(e.Type),
			RealmName: optionalString(e.RealmName), ClientID: optionalString(e.ClientID), Username: optionalString(e.Username),
			IPAddress: optionalString(e.IPAddress), Error: optionalString(e.Error),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
