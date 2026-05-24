package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*applicationResource)(nil)
	_ resource.ResourceWithConfigure   = (*applicationResource)(nil)
	_ resource.ResourceWithImportState = (*applicationResource)(nil)
)

type applicationResource struct {
	client *skycloak.Client
}

// NewApplicationResource returns the skycloak_application resource.
func NewApplicationResource() resource.Resource {
	return &applicationResource{}
}

type applicationModel struct {
	ID                    types.String `tfsdk:"id"`
	ClusterID             types.String `tfsdk:"cluster_id"`
	RealmName             types.String `tfsdk:"realm_name"`
	ClientID              types.String `tfsdk:"client_id"`
	Name                  types.String `tfsdk:"name"`
	Description           types.String `tfsdk:"description"`
	Type                  types.String `tfsdk:"type"`
	Protocol              types.String `tfsdk:"protocol"`
	Status                types.String `tfsdk:"status"`
	RedirectURIs          types.List   `tfsdk:"redirect_uris"`
	GrantTypes            types.List   `tfsdk:"grant_types"`
	PKCERequired          types.Bool   `tfsdk:"pkce_required"`
	ConsentRequired       types.Bool   `tfsdk:"consent_required"`
	ServiceAccountEnabled types.Bool   `tfsdk:"service_account_enabled"`
	ClientSecret          types.String `tfsdk:"client_secret"`
	CreatedAt             types.String `tfsdk:"created_at"`
	UpdatedAt             types.String `tfsdk:"updated_at"`
}

func (r *applicationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (r *applicationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An OIDC/SAML client (application) in a Skycloak realm.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/realm_name/client_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: requiresReplace},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: requiresReplace},
			"client_id":  schema.StringAttribute{Required: true, MarkdownDescription: "OAuth client ID (unique within the realm). Immutable.", PlanModifiers: requiresReplace},
			"name":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Display name.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description.",
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Client type: `confidential` or `public`.",
			},
			"protocol": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("openid-connect"),
				MarkdownDescription: "Protocol: `openid-connect` or `saml`. Immutable.",
				PlanModifiers:       requiresReplace,
			},
			"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Status (`active`/`inactive`)."},
			"redirect_uris": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Allowed redirect URIs.",
			},
			"grant_types": schema.ListAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "OAuth grant types enabled for the client.",
			},
			"pkce_required":           schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Require PKCE. Defaults to `false`."},
			"consent_required":        schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Require user consent. Defaults to `false`."},
			"service_account_enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Enable a service account (client credentials). Defaults to `false`."},
			"client_secret": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Client secret (confidential clients). Returned on create; use a rotate operation to change it.",
			},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
			"updated_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Last update timestamp."},
		},
	}
}

func (r *applicationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *applicationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, diags := applicationFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateApplication(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), app)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create application", err.Error())
		return
	}
	resp.Diagnostics.Append(applyApplicationToModel(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), created, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := r.client.GetApplication(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ClientID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read application", err.Error())
		return
	}
	// The API does not return the secret on read; preserve the stored value.
	app.ClientSecret = state.ClientSecret.ValueString()
	resp.Diagnostics.Append(applyApplicationToModel(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), app, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *applicationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan applicationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, diags := applicationFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateApplication(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), plan.ClientID.ValueString(), app)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update application", err.Error())
		return
	}
	if updated.ClientSecret == "" {
		updated.ClientSecret = plan.ClientSecret.ValueString()
	}
	resp.Diagnostics.Append(applyApplicationToModel(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), updated, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *applicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteApplication(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString(), state.ClientID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete application", err.Error())
	}
}

func (r *applicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name/client_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("client_id"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func applicationFromModel(ctx context.Context, m *applicationModel) (skycloak.Application, diag.Diagnostics) {
	var diags diag.Diagnostics
	app := skycloak.Application{
		ClientID:              m.ClientID.ValueString(),
		Name:                  m.Name.ValueString(),
		Description:           m.Description.ValueString(),
		Type:                  m.Type.ValueString(),
		Protocol:              m.Protocol.ValueString(),
		PKCERequired:          m.PKCERequired.ValueBool(),
		ConsentRequired:       m.ConsentRequired.ValueBool(),
		ServiceAccountEnabled: m.ServiceAccountEnabled.ValueBool(),
	}
	app.RedirectURIs = stringListToSlice(ctx, m.RedirectURIs, &diags)
	app.GrantTypes = stringListToSlice(ctx, m.GrantTypes, &diags)
	return app, diags
}

func applyApplicationToModel(ctx context.Context, clusterID, realm string, a *skycloak.Application, m *applicationModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(clusterID + "/" + realm + "/" + a.ClientID)
	m.ClusterID = types.StringValue(clusterID)
	m.RealmName = types.StringValue(realm)
	m.ClientID = types.StringValue(a.ClientID)
	m.Name = types.StringValue(a.Name)
	m.Description = optionalString(a.Description)
	m.Type = types.StringValue(a.Type)
	m.Protocol = types.StringValue(a.Protocol)
	m.Status = types.StringValue(a.Status)
	m.RedirectURIs = sliceToStringList(ctx, a.RedirectURIs, &diags)
	m.GrantTypes = sliceToStringList(ctx, a.GrantTypes, &diags)
	m.PKCERequired = types.BoolValue(a.PKCERequired)
	m.ConsentRequired = types.BoolValue(a.ConsentRequired)
	m.ServiceAccountEnabled = types.BoolValue(a.ServiceAccountEnabled)
	m.ClientSecret = types.StringValue(a.ClientSecret)
	m.CreatedAt = types.StringValue(a.CreatedAt)
	m.UpdatedAt = types.StringValue(a.UpdatedAt)
	return diags
}
