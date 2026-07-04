package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// siemDestinationDataModel is the read-only projection shared by the SIEM data
// sources. Secrets never appear; the has_* booleans stand in.
type siemDestinationDataModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Type            types.String `tfsdk:"type"`
	SourceType      types.String `tfsdk:"source_type"`
	HealthStatus    types.String `tfsdk:"health_status"`
	FailureCount    types.Int64  `tfsdk:"failure_count"`
	LastError       types.String `tfsdk:"last_error"`
	LastSentAt      types.String `tfsdk:"last_sent_at"`
	TotalEventsSent types.Int64  `tfsdk:"total_events_sent"`
	TotalLogsSent   types.Int64  `tfsdk:"total_logs_sent"`
	TotalBytesSent  types.Int64  `tfsdk:"total_bytes_sent"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

func siemDataAttributes(idRequired bool) map[string]schema.Attribute {
	id := schema.StringAttribute{Computed: true, MarkdownDescription: "Destination ID (UUID)."}
	if idRequired {
		id = schema.StringAttribute{Required: true, MarkdownDescription: "Destination ID (UUID)."}
	}
	return map[string]schema.Attribute{
		"id":                id,
		"name":              schema.StringAttribute{Computed: true, MarkdownDescription: "Destination name."},
		"enabled":           schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether forwarding is active."},
		"type":              schema.StringAttribute{Computed: true, MarkdownDescription: "Destination type: `syslog`, `s3`, or `http`."},
		"source_type":       schema.StringAttribute{Computed: true, MarkdownDescription: "Forwarded stream type."},
		"health_status":     schema.StringAttribute{Computed: true, MarkdownDescription: "`healthy`, `degraded`, or `failed`."},
		"failure_count":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Consecutive delivery failures."},
		"last_error":        schema.StringAttribute{Computed: true, MarkdownDescription: "Most recent delivery error."},
		"last_sent_at":      schema.StringAttribute{Computed: true, MarkdownDescription: "Last successful delivery."},
		"total_events_sent": schema.Int64Attribute{Computed: true},
		"total_logs_sent":   schema.Int64Attribute{Computed: true},
		"total_bytes_sent":  schema.Int64Attribute{Computed: true},
		"created_at":        schema.StringAttribute{Computed: true},
		"updated_at":        schema.StringAttribute{Computed: true},
	}
}

func applySIEMToDataModel(d *skycloak.SIEMDestination, m *siemDestinationDataModel) {
	m.ID = types.StringValue(d.ID)
	m.Name = types.StringValue(d.Name)
	m.Enabled = types.BoolValue(d.Enabled)
	m.Type = types.StringValue(d.Type)
	m.SourceType = types.StringValue(d.Source.Type)
	m.HealthStatus = types.StringValue(d.HealthStatus)
	m.FailureCount = types.Int64Value(d.FailureCount)
	m.LastError = optionalString(d.LastError)
	m.LastSentAt = optionalString(d.LastSentAt)
	m.TotalEventsSent = types.Int64Value(d.TotalEventsSent)
	m.TotalLogsSent = types.Int64Value(d.TotalLogsSent)
	m.TotalBytesSent = types.Int64Value(d.TotalBytesSent)
	m.CreatedAt = types.StringValue(d.CreatedAt)
	m.UpdatedAt = types.StringValue(d.UpdatedAt)
}

// ---- skycloak_siem_destination (single) ----

var (
	_ datasource.DataSource              = (*siemDestinationDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*siemDestinationDataSource)(nil)
)

type siemDestinationDataSource struct{ client *skycloak.Client }

// NewSIEMDestinationDataSource returns the skycloak_siem_destination data source.
func NewSIEMDestinationDataSource() datasource.DataSource { return &siemDestinationDataSource{} }

func (d *siemDestinationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_siem_destination"
}

func (d *siemDestinationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A single SIEM forwarding destination by ID.",
		Attributes:          siemDataAttributes(true),
	}
}

func (d *siemDestinationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*skycloak.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *skycloak.Client, got %T", req.ProviderData))
		return
	}
	d.client = client
}

func (d *siemDestinationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg siemDestinationDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	dest, err := d.client.GetSIEMDestination(ctx, cfg.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read SIEM destination", err.Error())
		return
	}
	applySIEMToDataModel(dest, &cfg)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// ---- skycloak_siem_destinations (list) ----

var (
	_ datasource.DataSource              = (*siemDestinationsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*siemDestinationsDataSource)(nil)
)

type siemDestinationsDataSource struct{ client *skycloak.Client }

// NewSIEMDestinationsDataSource returns the skycloak_siem_destinations data source.
func NewSIEMDestinationsDataSource() datasource.DataSource { return &siemDestinationsDataSource{} }

type siemDestinationsModel struct {
	Destinations []siemDestinationDataModel `tfsdk:"destinations"`
}

func (d *siemDestinationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_siem_destinations"
}

func (d *siemDestinationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "All SIEM forwarding destinations in the workspace.",
		Attributes: map[string]schema.Attribute{
			"destinations": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Destinations, with delivery health.",
				NestedObject:        schema.NestedAttributeObject{Attributes: siemDataAttributes(false)},
			},
		},
	}
}

func (d *siemDestinationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*skycloak.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *skycloak.Client, got %T", req.ProviderData))
		return
	}
	d.client = client
}

func (d *siemDestinationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	list, err := d.client.ListSIEMDestinations(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list SIEM destinations", err.Error())
		return
	}
	out := siemDestinationsModel{Destinations: make([]siemDestinationDataModel, 0, len(list))}
	for i := range list {
		var m siemDestinationDataModel
		applySIEMToDataModel(&list[i], &m)
		out.Destinations = append(out.Destinations, m)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}
