package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ datasource.DataSource              = (*clusterDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clusterDataSource)(nil)
)

type clusterDataSource struct {
	client *skycloak.Client
}

// NewClusterDataSource returns the skycloak_cluster data source.
func NewClusterDataSource() datasource.DataSource {
	return &clusterDataSource{}
}

func (d *clusterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (d *clusterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Skycloak cluster by ID.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID (UUID)."},
			"name":     schema.StringAttribute{Computed: true, MarkdownDescription: "Cluster name."},
			"type":     schema.StringAttribute{Computed: true, MarkdownDescription: "Cluster type."},
			"size":     schema.StringAttribute{Computed: true, MarkdownDescription: "Instance size."},
			"version":  schema.StringAttribute{Computed: true, MarkdownDescription: "Keycloak version."},
			"location": schema.StringAttribute{Computed: true, MarkdownDescription: "Region."},
			"status":   schema.StringAttribute{Computed: true, MarkdownDescription: "Lifecycle status."},
			"url":      schema.StringAttribute{Computed: true, MarkdownDescription: "Cluster base URL."},
			"auto_upgrade_enabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether automatic patch upgrades are enabled."},
		},
	}
}

func (d *clusterDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *clusterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state clusterModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cl, err := d.client.GetCluster(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read cluster", err.Error())
		return
	}

	applyClusterToModel(cl, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// compile-time assurance that the data source reuses the resource model fields.
var _ = types.StringNull
