package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*clusterPrivateLinkResource)(nil)
	_ resource.ResourceWithConfigure   = (*clusterPrivateLinkResource)(nil)
	_ resource.ResourceWithImportState = (*clusterPrivateLinkResource)(nil)
)

type clusterPrivateLinkResource struct{ client *skycloak.Client }

// NewClusterPrivateLinkResource returns the skycloak_cluster_private_link resource.
func NewClusterPrivateLinkResource() resource.Resource { return &clusterPrivateLinkResource{} }

type clusterPrivateLinkModel struct {
	ID                   types.String `tfsdk:"id"`
	ClusterID            types.String `tfsdk:"cluster_id"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	AzureSubscriptionIDs types.Set    `tfsdk:"azure_subscription_ids"`
	Hostnames            types.Set    `tfsdk:"hostnames"`
	Alias                types.String `tfsdk:"alias"`
}

func (r *clusterPrivateLinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_private_link"
}

func (r *clusterPrivateLinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Azure private link for a cluster. Only one resource should manage a cluster's private link. " +
			"Requires an Enterprise plan with private networking and a site that supports Azure private link. " +
			"Destroy disables private link and keeps the subscription IDs and hostnames. Restore public access before destroying this resource; " +
			"the API refuses to disable private link while public access is off. " +
			"The API key needs `clusters:private-link:read` and `clusters:private-link:write` scopes.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/private-link`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Cluster ID. Immutable.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{Required: true, MarkdownDescription: "Whether Azure private link is enabled."},
			"azure_subscription_ids": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Azure subscription IDs allowed to connect, as lowercase GUIDs (at most 20). At least one is required when enabled. An empty set clears the stored IDs.",
			},
			"hostnames": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				Validators:          []validator.Set{normalizedHostnamesValidator{}},
				MarkdownDescription: "Hostnames you reach the cluster on through your own proxy or DNS, as bare hosts: no scheme, port, path, query or credentials (at most 5). A host must be a DNS name with at least two labels. IP addresses, `localhost`, Skycloak domains and hosts another workspace already uses are refused. Write each host in lowercase with no surrounding spaces; other forms are refused at plan time. Removing this attribute clears the stored hostnames. They stay stored while private link is disabled.",
			},
			"alias": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Private Link Service alias to use when creating an Azure private endpoint. Null when disabled or not yet available.",
			},
		},
	}
}

func (r *clusterPrivateLinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*skycloak.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *skycloak.Client, got %T", req.ProviderData))
		return
	}
	r.client = client
}

// normalizedHostnamesValidator refuses hosts the API would lowercase or trim, because
// the value read back would then differ from the planned one.
type normalizedHostnamesValidator struct{}

func (normalizedHostnamesValidator) Description(context.Context) string {
	return "each host must be lowercase with no surrounding whitespace"
}

func (v normalizedHostnamesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (normalizedHostnamesValidator) ValidateSet(_ context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, el := range req.ConfigValue.Elements() {
		s, ok := el.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		if v := s.ValueString(); v != strings.ToLower(strings.TrimSpace(v)) {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid hostname",
				fmt.Sprintf("Host %q must be lowercase with no surrounding whitespace.", v))
		}
	}
}

func applyPrivateLinkToModel(ctx context.Context, link *skycloak.ClusterPrivateLink, m *clusterPrivateLinkModel) diag.Diagnostics {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/private-link")
	m.Enabled = types.BoolValue(link.Enabled)
	m.Alias = types.StringNull()
	if link.Alias != nil {
		m.Alias = optionalString(*link.Alias)
	}
	ids := link.AzureSubscriptionIDs
	if ids == nil {
		ids = []string{}
	}
	var diags diag.Diagnostics
	m.AzureSubscriptionIDs, diags = types.SetValueFrom(ctx, types.StringType, ids)
	if diags.HasError() {
		return diags
	}
	// No hostnames reads as null, except where the configuration holds an explicit empty set.
	if len(link.Hostnames) == 0 {
		if m.Hostnames.IsNull() || m.Hostnames.IsUnknown() || len(m.Hostnames.Elements()) != 0 {
			m.Hostnames = types.SetNull(types.StringType)
		}
		return diags
	}
	m.Hostnames, diags = types.SetValueFrom(ctx, types.StringType, link.Hostnames)
	return diags
}

func (r *clusterPrivateLinkResource) set(ctx context.Context, m *clusterPrivateLinkModel) diag.Diagnostics {
	var diags diag.Diagnostics
	var ids []string
	diags.Append(m.AzureSubscriptionIDs.ElementsAs(ctx, &ids, false)...)
	if diags.HasError() {
		return diags
	}
	if ids == nil {
		ids = []string{}
	}
	// A missing hostnames attribute sends an empty list, which clears what is stored.
	hostnames := []string{}
	if !m.Hostnames.IsNull() && !m.Hostnames.IsUnknown() {
		diags.Append(m.Hostnames.ElementsAs(ctx, &hostnames, false)...)
		if diags.HasError() {
			return diags
		}
	}
	link, err := r.client.UpdateClusterPrivateLink(ctx, m.ClusterID.ValueString(), m.Enabled.ValueBool(), ids, hostnames)
	if err != nil {
		diags.AddError("Unable to set cluster private link", err.Error())
		return diags
	}
	diags.Append(applyPrivateLinkToModel(ctx, link, m)...)
	return diags
}

func (r *clusterPrivateLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterPrivateLinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterPrivateLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterPrivateLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	link, err := r.client.GetClusterPrivateLink(ctx, state.ClusterID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read cluster private link", err.Error())
		return
	}
	resp.Diagnostics.Append(applyPrivateLinkToModel(ctx, link, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterPrivateLinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterPrivateLinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterPrivateLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterPrivateLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Omitting subscription IDs and hostnames preserves the stored lists when disabling the link.
	if _, err := r.client.UpdateClusterPrivateLink(ctx, state.ClusterID.ValueString(), false, nil, nil); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to disable cluster private link", err.Error())
	}
}

func (r *clusterPrivateLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID := strings.TrimSuffix(req.ID, "/private-link")
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), clusterID+"/private-link")...)
}
