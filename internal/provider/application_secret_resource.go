package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource              = (*applicationSecretResource)(nil)
	_ resource.ResourceWithConfigure = (*applicationSecretResource)(nil)
)

type applicationSecretResource struct {
	client *skycloak.Client
}

// NewApplicationSecretResource returns the skycloak_application_secret resource,
// which rotates an application's client secret. Changing `rotate_when` (or
// recreating the resource) generates a new secret.
func NewApplicationSecretResource() resource.Resource {
	return &applicationSecretResource{}
}

type applicationSecretModel struct {
	ID           types.String `tfsdk:"id"`
	ClusterID    types.String `tfsdk:"cluster_id"`
	RealmName    types.String `tfsdk:"realm_name"`
	ClientID     types.String `tfsdk:"client_id"`
	RotateWhen   types.Map    `tfsdk:"rotate_when"`
	ClientSecret types.String `tfsdk:"client_secret"`
}

func (r *applicationSecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_secret"
}

func (r *applicationSecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Rotates an application's client secret. The secret is regenerated whenever this resource is created or `rotate_when` changes.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Composite ID `cluster_id/realm_name/client_id`."},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rr},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: rr},
			"client_id":  schema.StringAttribute{Required: true, MarkdownDescription: "Application client ID. Immutable.", PlanModifiers: rr},
			"rotate_when": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Arbitrary key/value map; changing any value forces a new secret to be generated.",
				PlanModifiers:       []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"client_secret": schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "The current (rotated) client secret."},
		},
	}
}

func (r *applicationSecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *applicationSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	secret, err := r.client.RotateApplicationSecret(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.ClientID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to rotate application secret", err.Error())
		return
	}
	plan.ClientSecret = types.StringValue(secret)
	plan.ID = types.StringValue(plan.ClusterID.ValueString() + "/" + plan.RealmName.ValueString() + "/" + plan.ClientID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The secret can't be read back; just confirm the application still exists.
	if _, err := r.client.GetApplication(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ClientID.ValueString()); err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read application", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op: every configurable attribute forces replacement.
func (r *applicationSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan applicationSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state applicationSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	plan.ClientSecret = state.ClientSecret
	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: the secret lives on the application, not in this resource.
func (r *applicationSecretResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
