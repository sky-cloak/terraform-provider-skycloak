package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// openFGAInstanceTimeout bounds the wait for an instance to become ready, and
// separately the wait for it to be removed.
const openFGAInstanceTimeout = 20 * time.Minute

var (
	_ resource.Resource                = (*openFGAInstanceResource)(nil)
	_ resource.ResourceWithConfigure   = (*openFGAInstanceResource)(nil)
	_ resource.ResourceWithImportState = (*openFGAInstanceResource)(nil)
)

type openFGAInstanceResource struct{ client *skycloak.Client }

// NewOpenFGAInstanceResource returns the skycloak_openfga_instance resource.
func NewOpenFGAInstanceResource() resource.Resource { return &openFGAInstanceResource{} }

type openFGAInstanceModel struct {
	ID            types.String `tfsdk:"id"`
	ClusterID     types.String `tfsdk:"cluster_id"`
	Realm         types.String `tfsdk:"realm"`
	Status        types.String `tfsdk:"status"`
	StatusMessage types.String `tfsdk:"status_message"`
	APIURL        types.String `tfsdk:"api_url"`
	Issuer        types.String `tfsdk:"issuer"`
	Audience      types.String `tfsdk:"audience"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func applyOpenFGAInstanceToModel(i *skycloak.OpenFGAInstance, m *openFGAInstanceModel) {
	m.ID = types.StringValue(i.ID)
	m.ClusterID = types.StringValue(i.ClusterID)
	m.Realm = types.StringValue(i.Realm)
	m.Status = types.StringValue(i.Status)
	m.StatusMessage = types.StringValue(i.StatusMessage)
	m.APIURL = types.StringValue(i.APIURL)
	m.Issuer = types.StringValue(i.Issuer)
	m.Audience = types.StringValue(i.Audience)
	m.CreatedAt = types.StringValue(i.CreatedAt)
	m.UpdatedAt = types.StringValue(i.UpdatedAt)
}

func (r *openFGAInstanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_openfga_instance"
}

func (r *openFGAInstanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A dedicated OpenFGA instance bound to one realm of a cluster. The instance's API accepts only " +
			"tokens issued by that realm. Creation waits until the instance is `ready`. Destroying the instance deletes " +
			"all of its authorization data and cannot be undone. Changing `cluster_id` or `realm` replaces the instance. " +
			"Requires a workspace with OpenFGA access. A realm can have only one instance.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Instance ID (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Changing it replaces the instance.", PlanModifiers: rr},
			"realm":      schema.StringAttribute{Required: true, MarkdownDescription: "Realm to bind. It must exist and be enabled on the cluster and cannot be `master`. Changing it replaces the instance.", PlanModifiers: rr},
			"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "Lifecycle status: `provisioning`, `ready`, `failed` or `deleting`."},
			"status_message": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The reason the instance is `failed`; empty otherwise.",
			},
			"api_url":    schema.StringAttribute{Computed: true, MarkdownDescription: "Where the instance's OpenFGA API answers."},
			"issuer":     schema.StringAttribute{Computed: true, MarkdownDescription: "The issuer the instance trusts. Only tokens from this issuer are accepted."},
			"audience":   schema.StringAttribute{Computed: true, MarkdownDescription: "The audience a token must carry. Add an audience mapper for this value to the realm's client."},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
			"updated_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Last update timestamp."},
		},
	}
}

func (r *openFGAInstanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *openFGAInstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan openFGAInstanceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ClusterID.ValueString()
	created, err := r.client.CreateOpenFGAInstance(ctx, clusterID, plan.Realm.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create OpenFGA instance", err.Error())
		return
	}

	// Creation is async (202). Poll until ready, bounded by openFGAInstanceTimeout.
	waitCtx, cancel := context.WithTimeout(ctx, openFGAInstanceTimeout)
	defer cancel()
	final, err := r.client.WaitForOpenFGAInstanceReady(waitCtx, clusterID, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("OpenFGA instance did not become ready", err.Error())
		// Persist what is known so the half-created instance is tracked, not leaked.
		if final == nil {
			final = created
		}
		applyOpenFGAInstanceToModel(final, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	applyOpenFGAInstanceToModel(final, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *openFGAInstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state openFGAInstanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	inst, err := r.client.GetOpenFGAInstance(ctx, state.ClusterID.ValueString(), state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read OpenFGA instance", err.Error())
		return
	}
	applyOpenFGAInstanceToModel(inst, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *openFGAInstanceResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Both configurable attributes force replacement; Update is unreachable.
	resp.Diagnostics.AddError("Unsupported update", "skycloak_openfga_instance has no updatable attributes")
}

func (r *openFGAInstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state openFGAInstanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID, id := state.ClusterID.ValueString(), state.ID.ValueString()
	if err := r.client.DeleteOpenFGAInstance(ctx, clusterID, id); err != nil {
		if skycloak.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Unable to delete OpenFGA instance", err.Error())
		return
	}

	// Removal is async (202). Wait until the instance is gone: a replacement
	// created right after would otherwise meet the old instance on the same realm.
	waitCtx, cancel := context.WithTimeout(ctx, openFGAInstanceTimeout)
	defer cancel()
	if err := r.client.WaitForOpenFGAInstanceGone(waitCtx, clusterID, id); err != nil {
		resp.Diagnostics.AddError("OpenFGA instance was not removed", err.Error())
	}
}

func (r *openFGAInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected `cluster_id/instance_id`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
