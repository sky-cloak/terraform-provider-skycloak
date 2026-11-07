package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ datasource.DataSource              = (*openFGAInstanceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*openFGAInstanceDataSource)(nil)
)

type openFGAInstanceDataSource struct{ client *skycloak.Client }

// NewOpenFGAInstanceDataSource returns the skycloak_openfga_instance data source.
func NewOpenFGAInstanceDataSource() datasource.DataSource { return &openFGAInstanceDataSource{} }

func (d *openFGAInstanceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_openfga_instance"
}

func (d *openFGAInstanceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing OpenFGA instance on a cluster, by instance ID or by the realm it is bound to.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"id":         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Instance ID (UUID). Set either `id` or `realm`."},
			"realm":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Realm the instance is bound to. Set either `id` or `realm`."},
			"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "Lifecycle status: `provisioning`, `ready`, `failed` or `deleting`."},
			"status_message": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The reason the instance is `failed`; empty otherwise.",
			},
			"api_url":    schema.StringAttribute{Computed: true, MarkdownDescription: "Where the instance's OpenFGA API answers."},
			"issuer":     schema.StringAttribute{Computed: true, MarkdownDescription: "The issuer the instance trusts."},
			"audience":   schema.StringAttribute{Computed: true, MarkdownDescription: "The audience a token must carry."},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
			"updated_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Last update timestamp."},
		},
	}
}

func (d *openFGAInstanceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *openFGAInstanceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state openFGAInstanceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ID.IsNull() == state.Realm.IsNull() {
		resp.Diagnostics.AddError("Invalid lookup", "set exactly one of `id` or `realm`")
		return
	}

	clusterID := state.ClusterID.ValueString()
	var inst *skycloak.OpenFGAInstance
	if !state.ID.IsNull() {
		got, err := d.client.GetOpenFGAInstance(ctx, clusterID, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read OpenFGA instance", err.Error())
			return
		}
		inst = got
	} else {
		list, err := d.client.ListOpenFGAInstances(ctx, clusterID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to list OpenFGA instances", err.Error())
			return
		}
		for i := range list {
			if list[i].Realm == state.Realm.ValueString() {
				inst = &list[i]
				break
			}
		}
		if inst == nil {
			resp.Diagnostics.AddError("OpenFGA instance not found",
				fmt.Sprintf("cluster %s has no OpenFGA instance bound to realm %q", clusterID, state.Realm.ValueString()))
			return
		}
	}

	applyOpenFGAInstanceToModel(inst, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
