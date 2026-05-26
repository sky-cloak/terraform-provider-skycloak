package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ datasource.DataSource              = (*clusterEventsExportDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterEventsExportDataSource)(nil)
)

type clusterEventsExportDataSource struct{ client *skycloak.Client }

// NewClusterEventsExportDataSource returns the skycloak_cluster_events_export data source.
func NewClusterEventsExportDataSource() datasource.DataSource {
	return &clusterEventsExportDataSource{}
}

type clusterEventsExportModel struct {
	ClusterID types.String `tfsdk:"cluster_id"`
	Document  types.String `tfsdk:"document"`
}

func (d *clusterEventsExportDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_events_export"
}

func (d *clusterEventsExportDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Exports a cluster's events as a document (the raw export body).",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"document":   schema.StringAttribute{Computed: true, MarkdownDescription: "The exported events document."},
		},
	}
}

func (d *clusterEventsExportDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configuredClient(req, &resp.Diagnostics)
}

func (d *clusterEventsExportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg clusterEventsExportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	raw, err := d.client.ExportClusterEvents(ctx, cfg.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to export cluster events", err.Error())
		return
	}
	cfg.Document = types.StringValue(string(raw))
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
