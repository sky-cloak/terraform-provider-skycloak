package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// dnsRecordObjectType is the element type of domainModel.DNSRecords. A plain Go
// slice can't represent an Unknown container, so this Computed-only list must
// be modeled as types.List (unlike data-source-only lists elsewhere in this
// package, which never see an Unknown planned value).
var dnsRecordObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"type": types.StringType, "name": types.StringType, "value": types.StringType,
}}

var (
	_ resource.Resource                = (*domainResource)(nil)
	_ resource.ResourceWithConfigure   = (*domainResource)(nil)
	_ resource.ResourceWithImportState = (*domainResource)(nil)
)

type domainResource struct{ client *skycloak.Client }

// NewDomainResource returns the skycloak_domain resource.
func NewDomainResource() resource.Resource { return &domainResource{} }

type domainModel struct {
	ID                 types.String     `tfsdk:"id"`
	ClusterID          types.String     `tfsdk:"cluster_id"`
	Domain             types.String     `tfsdk:"domain"`
	Subdomain          types.String     `tfsdk:"subdomain"`
	CnameTarget        types.String     `tfsdk:"cname_target"`
	SSLStatus          types.String     `tfsdk:"ssl_status"`
	VerificationStatus types.String     `tfsdk:"verification_status"`
	IsActive           types.Bool       `tfsdk:"is_active"`
	DNSRecords         types.List       `tfsdk:"dns_records"`
	CreatedAt          types.String     `tfsdk:"created_at"`
	UpdatedAt          types.String     `tfsdk:"updated_at"`
}

type dnsRecordModel struct {
	Type  types.String `tfsdk:"type"`
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

func (r *domainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (r *domainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A custom domain on a Skycloak cluster. After apply, create the returned `dns_records` at your DNS provider; verification then completes out of band (refresh to observe `verification_status`).",
		Attributes: map[string]schema.Attribute{
			"id":                  schema.StringAttribute{Computed: true, MarkdownDescription: "Domain ID (UUID).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"cluster_id":          schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rr},
			"domain":              schema.StringAttribute{Required: true, MarkdownDescription: "Fully-qualified domain name. Immutable.", PlanModifiers: rr},
			"subdomain":           schema.StringAttribute{Optional: true, MarkdownDescription: "Optional subdomain. Immutable.", PlanModifiers: rr},
			"cname_target":        schema.StringAttribute{Computed: true, MarkdownDescription: "CNAME target to point the domain at."},
			"ssl_status":          schema.StringAttribute{Computed: true, MarkdownDescription: "SSL provisioning status."},
			"verification_status": schema.StringAttribute{Computed: true, MarkdownDescription: "DNS verification status."},
			"is_active":           schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the domain is active."},
			"dns_records": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "DNS records to create for verification/routing.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type":  schema.StringAttribute{Computed: true},
						"name":  schema.StringAttribute{Computed: true},
						"value": schema.StringAttribute{Computed: true},
					},
				},
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *domainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *domainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.CreateDomain(ctx, plan.ClusterID.ValueString(), skycloak.CreateDomainRequest{
		Domain: plan.Domain.ValueString(), Subdomain: plan.Subdomain.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create domain", err.Error())
		return
	}
	resp.Diagnostics.Append(applyDomainToModel(ctx, d, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDomain(ctx, state.ClusterID.ValueString(), state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read domain", err.Error())
		return
	}
	resp.Diagnostics.Append(applyDomainToModel(ctx, d, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable (all configurable attributes force replacement) but
// required by the interface.
func (r *domainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan domainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDomain(ctx, state.ClusterID.ValueString(), state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete domain", err.Error())
	}
}

func (r *domainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID, domainID, ok := strings.Cut(req.ID, "/")
	if !ok || clusterID == "" || domainID == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/domain_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), domainID)...)
}

func applyDomainToModel(ctx context.Context, d *skycloak.Domain, m *domainModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(d.ID)
	m.ClusterID = types.StringValue(d.ClusterID)
	m.Domain = types.StringValue(d.Domain)
	m.Subdomain = optionalString(d.Subdomain)
	m.CnameTarget = types.StringValue(d.CnameTarget)
	m.SSLStatus = types.StringValue(d.SSLStatus)
	m.VerificationStatus = types.StringValue(d.VerificationStatus)
	m.IsActive = types.BoolValue(d.IsActive)
	m.CreatedAt = types.StringValue(d.CreatedAt)
	m.UpdatedAt = types.StringValue(d.UpdatedAt)

	records := make([]dnsRecordModel, 0, len(d.DNSRecords))
	for _, rec := range d.DNSRecords {
		records = append(records, dnsRecordModel{
			Type: types.StringValue(rec.Type), Name: types.StringValue(rec.Name), Value: types.StringValue(rec.Value),
		})
	}
	list, d2 := types.ListValueFrom(ctx, dnsRecordObjectType, records)
	diags.Append(d2...)
	m.DNSRecords = list
	return diags
}
