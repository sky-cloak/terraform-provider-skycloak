package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// ---- skycloak_webhook_subscriptions ----

var (
	_ datasource.DataSource              = (*webhookSubscriptionsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*webhookSubscriptionsDataSource)(nil)
)

type webhookSubscriptionsDataSource struct{ client *skycloak.Client }

// NewWebhookSubscriptionsDataSource returns the skycloak_webhook_subscriptions data source.
func NewWebhookSubscriptionsDataSource() datasource.DataSource {
	return &webhookSubscriptionsDataSource{}
}

type webhookSubscriptionDataModel struct {
	ID                     types.String   `tfsdk:"id"`
	Name                   types.String   `tfsdk:"name"`
	URL                    types.String   `tfsdk:"url"`
	Enabled                types.Bool     `tfsdk:"enabled"`
	Source                 types.String   `tfsdk:"source"`
	EventTypes             []types.String `tfsdk:"event_types"`
	ClusterID              types.String   `tfsdk:"cluster_id"`
	RealmID                types.String   `tfsdk:"realm_id"`
	HasAuthorizationHeader types.Bool     `tfsdk:"has_authorization_header"`
	HasSigningSecret       types.Bool     `tfsdk:"has_signing_secret"`
	CreatedAt              types.String   `tfsdk:"created_at"`
	UpdatedAt              types.String   `tfsdk:"updated_at"`
}

type webhookSubscriptionsModel struct {
	Subscriptions []webhookSubscriptionDataModel `tfsdk:"subscriptions"`
}

func (d *webhookSubscriptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_subscriptions"
}

func (d *webhookSubscriptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "All webhook subscriptions in the workspace.",
		Attributes: map[string]schema.Attribute{
			"subscriptions": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Subscription ID (UUID)."},
						"name":    schema.StringAttribute{Computed: true},
						"url":     schema.StringAttribute{Computed: true},
						"enabled": schema.BoolAttribute{Computed: true},
						"source":  schema.StringAttribute{Computed: true, MarkdownDescription: "`keycloak` or `platform`."},
						"event_types": schema.ListAttribute{
							Computed: true, ElementType: types.StringType,
						},
						"cluster_id":               schema.StringAttribute{Computed: true, MarkdownDescription: "Cluster scoping, if any."},
						"realm_id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Realm scoping, if any."},
						"has_authorization_header": schema.BoolAttribute{Computed: true},
						"has_signing_secret":       schema.BoolAttribute{Computed: true},
						"created_at":               schema.StringAttribute{Computed: true},
						"updated_at":               schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *webhookSubscriptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *webhookSubscriptionsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	list, err := d.client.ListWebhookSubscriptions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list webhook subscriptions", err.Error())
		return
	}
	out := webhookSubscriptionsModel{Subscriptions: make([]webhookSubscriptionDataModel, 0, len(list))}
	for _, w := range list {
		m := webhookSubscriptionDataModel{
			ID: types.StringValue(w.ID), Name: types.StringValue(w.Name), URL: types.StringValue(w.URL),
			Enabled: types.BoolValue(w.Enabled), Source: types.StringValue(w.Source),
			ClusterID: optionalString(w.ClusterID), RealmID: optionalString(w.RealmID),
			HasAuthorizationHeader: types.BoolValue(w.HasAuthorizationHeader),
			HasSigningSecret:       types.BoolValue(w.HasSigningSecret),
			CreatedAt:              types.StringValue(w.CreatedAt), UpdatedAt: types.StringValue(w.UpdatedAt),
		}
		for _, e := range w.EventTypes {
			m.EventTypes = append(m.EventTypes, types.StringValue(e))
		}
		out.Subscriptions = append(out.Subscriptions, m)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}

// ---- skycloak_webhook_event_types ----

var (
	_ datasource.DataSource              = (*webhookEventTypesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*webhookEventTypesDataSource)(nil)
)

type webhookEventTypesDataSource struct{ client *skycloak.Client }

// NewWebhookEventTypesDataSource returns the skycloak_webhook_event_types data source.
func NewWebhookEventTypesDataSource() datasource.DataSource { return &webhookEventTypesDataSource{} }

type webhookEventTypeModel struct {
	Type        types.String `tfsdk:"type"`
	Category    types.String `tfsdk:"category"`
	Description types.String `tfsdk:"description"`
	Deprecated  types.Bool   `tfsdk:"deprecated"`
}

type webhookEventTypesModel struct {
	EventTypes []webhookEventTypeModel `tfsdk:"event_types"`
}

func (d *webhookEventTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_event_types"
}

func (d *webhookEventTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The catalog of event types webhook subscriptions can deliver.",
		Attributes: map[string]schema.Attribute{
			"event_types": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type":        schema.StringAttribute{Computed: true, MarkdownDescription: "Event type code."},
						"category":    schema.StringAttribute{Computed: true, MarkdownDescription: "Event category."},
						"description": schema.StringAttribute{Computed: true},
						"deprecated":  schema.BoolAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *webhookEventTypesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *webhookEventTypesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	list, err := d.client.ListWebhookEventTypes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list webhook event types", err.Error())
		return
	}
	out := webhookEventTypesModel{EventTypes: make([]webhookEventTypeModel, 0, len(list))}
	for _, e := range list {
		out.EventTypes = append(out.EventTypes, webhookEventTypeModel{
			Type: types.StringValue(e.Type), Category: types.StringValue(e.Category),
			Description: types.StringValue(e.Description), Deprecated: types.BoolValue(e.Deprecated),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}
