package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*captchaDomainResource)(nil)
	_ resource.ResourceWithConfigure   = (*captchaDomainResource)(nil)
	_ resource.ResourceWithImportState = (*captchaDomainResource)(nil)
)

type captchaDomainResource struct{ client *skycloak.Client }

// NewCAPTCHADomainResource returns the skycloak_captcha_domain resource.
func NewCAPTCHADomainResource() resource.Resource { return &captchaDomainResource{} }

type captchaDomainModel struct {
	ID        types.String `tfsdk:"id"`
	ClusterID types.String `tfsdk:"cluster_id"`
	Hostname  types.String `tfsdk:"hostname"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (r *captchaDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_captcha_domain"
}

func (r *captchaDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A hostname registered for CAPTCHA protection on a cluster. The number of registrable " +
			"hostnames is capped per cluster; the API rejects additions beyond the cap.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/hostname`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rr},
			"hostname":   schema.StringAttribute{Required: true, MarkdownDescription: "Hostname to protect. Immutable.", PlanModifiers: rr},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Registration timestamp."},
		},
	}
}

func (r *captchaDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *captchaDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan captchaDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.AddCAPTCHADomain(ctx, plan.ClusterID.ValueString(), plan.Hostname.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to register CAPTCHA domain", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.ClusterID.ValueString() + "/" + d.Hostname)
	plan.CreatedAt = types.StringValue(d.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *captchaDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state captchaDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	list, err := r.client.ListCAPTCHADomains(ctx, state.ClusterID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read CAPTCHA domains", err.Error())
		return
	}
	for _, d := range list.Domains {
		if d.Hostname == state.Hostname.ValueString() {
			state.ID = types.StringValue(state.ClusterID.ValueString() + "/" + d.Hostname)
			state.CreatedAt = types.StringValue(d.CreatedAt)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *captchaDomainResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Both attributes force replacement; Update is unreachable.
	resp.Diagnostics.AddError("Unsupported update", "skycloak_captcha_domain has no updatable attributes")
}

func (r *captchaDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state captchaDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RemoveCAPTCHADomain(ctx, state.ClusterID.ValueString(), state.Hostname.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to remove CAPTCHA domain", err.Error())
	}
}

func (r *captchaDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected `cluster_id/hostname`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("hostname"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
