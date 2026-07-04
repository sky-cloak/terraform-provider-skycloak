package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*webhookSubscriptionResource)(nil)
	_ resource.ResourceWithConfigure   = (*webhookSubscriptionResource)(nil)
	_ resource.ResourceWithImportState = (*webhookSubscriptionResource)(nil)
)

type webhookSubscriptionResource struct{ client *skycloak.Client }

// NewWebhookSubscriptionResource returns the skycloak_webhook_subscription resource.
func NewWebhookSubscriptionResource() resource.Resource { return &webhookSubscriptionResource{} }

type webhookSubscriptionModel struct {
	ID                     types.String   `tfsdk:"id"`
	Name                   types.String   `tfsdk:"name"`
	URL                    types.String   `tfsdk:"url"`
	Enabled                types.Bool     `tfsdk:"enabled"`
	Source                 types.String   `tfsdk:"source"`
	EventTypes             []types.String `tfsdk:"event_types"`
	ClusterID              types.String   `tfsdk:"cluster_id"`
	RealmID                types.String   `tfsdk:"realm_id"`
	SigningSecret          types.String   `tfsdk:"signing_secret"`
	AuthorizationHeader    types.String   `tfsdk:"authorization_header"`
	HasAuthorizationHeader types.Bool     `tfsdk:"has_authorization_header"`
	HasSigningSecret       types.Bool     `tfsdk:"has_signing_secret"`
	CreatedAt              types.String   `tfsdk:"created_at"`
	UpdatedAt              types.String   `tfsdk:"updated_at"`
}

func (r *webhookSubscriptionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_subscription"
}

func (r *webhookSubscriptionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A webhook subscription delivering workspace events to an HTTPS endpoint. " +
			"`signing_secret` and `authorization_header` are write-only: the API never returns them and " +
			"reads report `has_*` booleans instead.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Subscription ID (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Subscription name."},
			"url":     schema.StringAttribute{Required: true, MarkdownDescription: "HTTPS endpoint receiving deliveries."},
			"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether deliveries are active. Defaults to `true`."},
			"source":  schema.StringAttribute{Required: true, MarkdownDescription: "Event source: `keycloak` or `platform`."},
			"event_types": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Event types to deliver. See the `skycloak_webhook_event_types` data source for the catalog.",
			},
			"cluster_id": schema.StringAttribute{Optional: true, MarkdownDescription: "Scope deliveries to one cluster. Omit for all clusters."},
			"realm_id":   schema.StringAttribute{Optional: true, MarkdownDescription: "Scope deliveries to one Keycloak realm ID (UUID)."},
			"signing_secret": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "HMAC secret used to sign deliveries, 32 to 512 characters. Write-only; never read back.",
			},
			"authorization_header": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Authorization header value sent with each delivery. Write-only; omit to retain the stored value.",
			},
			"has_authorization_header": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether an authorization header is stored."},
			"has_signing_secret":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a signing secret is stored."},
			"created_at":               schema.StringAttribute{Computed: true},
			"updated_at":               schema.StringAttribute{Computed: true},
		},
	}
}

func (r *webhookSubscriptionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (m *webhookSubscriptionModel) toRequest() skycloak.WebhookSubscriptionRequest {
	enabled := m.Enabled.ValueBool()
	return skycloak.WebhookSubscriptionRequest{
		Name:                m.Name.ValueString(),
		URL:                 m.URL.ValueString(),
		Enabled:             &enabled,
		Source:              m.Source.ValueString(),
		EventTypes:          strList(m.EventTypes),
		ClusterID:           m.ClusterID.ValueString(),
		RealmID:             m.RealmID.ValueString(),
		SigningSecret:       m.SigningSecret.ValueString(),
		AuthorizationHeader: m.AuthorizationHeader.ValueString(),
	}
}

// applyToModel maps API state onto the model, keeping the write-only secret
// values already present (the API never returns them).
func (m *webhookSubscriptionModel) applyToModel(w *skycloak.WebhookSubscription) {
	m.ID = types.StringValue(w.ID)
	m.Name = types.StringValue(w.Name)
	m.URL = types.StringValue(w.URL)
	m.Enabled = types.BoolValue(w.Enabled)
	m.Source = types.StringValue(w.Source)
	m.EventTypes = make([]types.String, 0, len(w.EventTypes))
	for _, e := range w.EventTypes {
		m.EventTypes = append(m.EventTypes, types.StringValue(e))
	}
	m.ClusterID = optionalString(w.ClusterID)
	m.RealmID = optionalString(w.RealmID)
	m.HasAuthorizationHeader = types.BoolValue(w.HasAuthorizationHeader)
	m.HasSigningSecret = types.BoolValue(w.HasSigningSecret)
	m.CreatedAt = types.StringValue(w.CreatedAt)
	m.UpdatedAt = types.StringValue(w.UpdatedAt)
}

func (r *webhookSubscriptionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookSubscriptionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.CreateWebhookSubscription(ctx, plan.toRequest())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create webhook subscription", err.Error())
		return
	}
	plan.applyToModel(w)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookSubscriptionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookSubscriptionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.GetWebhookSubscription(ctx, state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read webhook subscription", err.Error())
		return
	}
	state.applyToModel(w)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webhookSubscriptionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookSubscriptionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state webhookSubscriptionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.UpdateWebhookSubscription(ctx, state.ID.ValueString(), plan.toRequest())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update webhook subscription", err.Error())
		return
	}
	plan.applyToModel(w)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookSubscriptionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookSubscriptionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteWebhookSubscription(ctx, state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete webhook subscription", err.Error())
	}
}

func (r *webhookSubscriptionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
