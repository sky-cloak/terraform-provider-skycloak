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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*domainRouteResource)(nil)
	_ resource.ResourceWithConfigure   = (*domainRouteResource)(nil)
	_ resource.ResourceWithImportState = (*domainRouteResource)(nil)
)

type domainRouteResource struct{ client *skycloak.Client }

// NewDomainRouteResource returns the skycloak_domain_route resource.
func NewDomainRouteResource() resource.Resource { return &domainRouteResource{} }

type domainRouteModel struct {
	ID                 types.String `tfsdk:"id"`
	ClusterID          types.String `tfsdk:"cluster_id"`
	DomainID           types.String `tfsdk:"domain_id"`
	Realm              types.String `tfsdk:"realm"`
	AllowAdminAccess   types.Bool   `tfsdk:"allow_admin_access"`
	HideRealmPath      types.Bool   `tfsdk:"hide_realm_path"`
	CorsAllowedOrigins types.List   `tfsdk:"cors_allowed_origins"`
	CreatedAt          types.String `tfsdk:"created_at"`
	UpdatedAt          types.String `tfsdk:"updated_at"`
}

func (r *domainRouteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain_route"
}

func (r *domainRouteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Maps a realm onto a custom domain.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Route ID (UUID).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"cluster_id":         schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rr},
			"domain_id":          schema.StringAttribute{Required: true, MarkdownDescription: "Custom domain ID. Immutable.", PlanModifiers: rr},
			"realm":              schema.StringAttribute{Required: true, MarkdownDescription: "Realm to route. Immutable.", PlanModifiers: rr},
			"allow_admin_access": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Allow admin console access through this route. Defaults to `false`."},
			"hide_realm_path": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Serve the realm at the domain root (hide `/realms/<name>`). Set at creation; immutable.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"cors_allowed_origins": schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "CORS allowed origins for this route."},
			"created_at":           schema.StringAttribute{Computed: true},
			"updated_at":           schema.StringAttribute{Computed: true},
		},
	}
}

func (r *domainRouteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *domainRouteResource) input(ctx context.Context, m *domainRouteModel, diags *diag.Diagnostics) skycloak.DomainRouteInput {
	return skycloak.DomainRouteInput{
		Realm:              m.Realm.ValueString(),
		AllowAdminAccess:   m.AllowAdminAccess.ValueBool(),
		HideRealmPath:      m.HideRealmPath.ValueBool(),
		CorsAllowedOrigins: stringListToSlice(ctx, m.CorsAllowedOrigins, diags),
	}
}

func (r *domainRouteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainRouteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := r.input(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	route, err := r.client.CreateDomainRoute(ctx, plan.ClusterID.ValueString(), plan.DomainID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create domain route", err.Error())
		return
	}
	resp.Diagnostics.Append(applyDomainRouteToModel(ctx, route, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainRouteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state domainRouteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	route, err := r.client.GetDomainRoute(ctx, state.ClusterID.ValueString(), state.DomainID.ValueString(), state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read domain route", err.Error())
		return
	}
	resp.Diagnostics.Append(applyDomainRouteToModel(ctx, route, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *domainRouteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan domainRouteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := r.input(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	route, err := r.client.UpdateDomainRoute(ctx, plan.ClusterID.ValueString(), plan.DomainID.ValueString(), plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update domain route", err.Error())
		return
	}
	resp.Diagnostics.Append(applyDomainRouteToModel(ctx, route, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainRouteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state domainRouteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDomainRoute(ctx, state.ClusterID.ValueString(), state.DomainID.ValueString(), state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete domain route", err.Error())
	}
}

func (r *domainRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/domain_id/route_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}

func applyDomainRouteToModel(ctx context.Context, route *skycloak.DomainRoute, m *domainRouteModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(route.ID)
	m.ClusterID = types.StringValue(route.ClusterID)
	m.DomainID = types.StringValue(route.DomainID)
	m.Realm = types.StringValue(route.Realm)
	m.AllowAdminAccess = types.BoolValue(route.AllowAdminAccess)
	m.HideRealmPath = types.BoolValue(route.HideRealmPath)
	m.CorsAllowedOrigins = sliceToStringList(ctx, route.CorsAllowedOrigins, &diags)
	m.CreatedAt = types.StringValue(route.CreatedAt)
	m.UpdatedAt = types.StringValue(route.UpdatedAt)
	return diags
}
