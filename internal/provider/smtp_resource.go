package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*smtpResource)(nil)
	_ resource.ResourceWithConfigure   = (*smtpResource)(nil)
	_ resource.ResourceWithImportState = (*smtpResource)(nil)
)

type smtpResource struct {
	client *skycloak.Client
}

// NewSMTPResource returns the skycloak_smtp resource (realm SMTP configuration).
func NewSMTPResource() resource.Resource {
	return &smtpResource{}
}

type smtpModel struct {
	ID              types.String `tfsdk:"id"`
	ClusterID       types.String `tfsdk:"cluster_id"`
	RealmName       types.String `tfsdk:"realm_name"`
	Host            types.String `tfsdk:"host"`
	Port            types.Int64  `tfsdk:"port"`
	Encryption      types.String `tfsdk:"encryption"`
	FromEmail       types.String `tfsdk:"from_email"`
	FromName        types.String `tfsdk:"from_name"`
	AuthType        types.String `tfsdk:"auth_type"`
	Username        types.String `tfsdk:"username"`
	Password        types.String `tfsdk:"password"`
	HasPassword     types.Bool   `tfsdk:"has_password"`
	TokenURL        types.String `tfsdk:"token_url"`
	TokenScope      types.String `tfsdk:"token_scope"`
	ClientID        types.String `tfsdk:"client_id"`
	ClientSecret    types.String `tfsdk:"client_secret"`
	HasClientSecret types.Bool   `tfsdk:"has_client_secret"`
	Status          types.String `tfsdk:"status"`
}

func (r *smtpResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_smtp"
}

func (r *smtpResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "SMTP configuration for a realm (a singleton; create == update upsert).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/realm_name/smtp`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: requiresReplace},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: requiresReplace},
			"host":       schema.StringAttribute{Required: true, MarkdownDescription: "SMTP server hostname or IP."},
			"port":       schema.Int64Attribute{Required: true, MarkdownDescription: "SMTP server port (1-65535), e.g. 587."},
			"encryption": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("none"),
				MarkdownDescription: "Encryption mode: `none`, `ssl`, or `starttls`. Defaults to `none`.",
			},
			"from_email": schema.StringAttribute{Required: true, MarkdownDescription: "Sender email address."},
			"from_name":  schema.StringAttribute{Optional: true, MarkdownDescription: "Sender display name."},
			"auth_type":  schema.StringAttribute{Required: true, MarkdownDescription: "Authentication type: `basic` or `oauth2`."},
			"username":   schema.StringAttribute{Optional: true, MarkdownDescription: "SMTP username (when `auth_type` is `basic`)."},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "SMTP password (when `auth_type` is `basic`). Write-only; never read back. Omit to retain the stored value.",
			},
			"has_password": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a password is stored."},
			"token_url":    schema.StringAttribute{Optional: true, MarkdownDescription: "OAuth 2.0 token endpoint (when `auth_type` is `oauth2`)."},
			"token_scope":  schema.StringAttribute{Optional: true, MarkdownDescription: "OAuth 2.0 token scope (when `auth_type` is `oauth2`)."},
			"client_id":    schema.StringAttribute{Optional: true, MarkdownDescription: "OAuth 2.0 client ID (when `auth_type` is `oauth2`)."},
			"client_secret": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "OAuth 2.0 client secret. Write-only; never read back. Omit to retain the stored value.",
			},
			"has_client_secret": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a client secret is stored."},
			"status":            schema.StringAttribute{Computed: true, MarkdownDescription: "Configuration status (`configured`, `verified`, `failed`)."},
		},
	}
}

func (r *smtpResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *smtpResource) upsert(ctx context.Context, plan *smtpModel) (*skycloak.SMTPConfig, error) {
	return r.client.UpsertSMTP(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), skycloak.UpsertSMTPRequest{
		Host:         plan.Host.ValueString(),
		Port:         plan.Port.ValueInt64(),
		Encryption:   plan.Encryption.ValueString(),
		FromEmail:    plan.FromEmail.ValueString(),
		FromName:     plan.FromName.ValueString(),
		AuthType:     plan.AuthType.ValueString(),
		Username:     plan.Username.ValueString(),
		Password:     plan.Password.ValueString(),
		TokenURL:     plan.TokenURL.ValueString(),
		TokenScope:   plan.TokenScope.ValueString(),
		ClientID:     plan.ClientID.ValueString(),
		ClientSecret: plan.ClientSecret.ValueString(),
	})
}

func (r *smtpResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan smtpModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.upsert(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Unable to configure SMTP", err.Error())
		return
	}
	applySMTPToModel(cfg, &plan) // preserves write-only password/client_secret from plan
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *smtpResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state smtpModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.GetSMTP(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read SMTP config", err.Error())
		return
	}
	applySMTPToModel(cfg, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *smtpResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan smtpModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.upsert(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update SMTP config", err.Error())
		return
	}
	applySMTPToModel(cfg, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *smtpResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state smtpModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSMTP(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete SMTP config", err.Error())
	}
}

func (r *smtpResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[0]+"/"+parts[1]+"/smtp")...)
}

// applySMTPToModel copies API (non-secret) fields into the model. The write-only
// password / client_secret already in the model are left untouched, since the
// API never returns them.
func applySMTPToModel(c *skycloak.SMTPConfig, m *smtpModel) {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/" + m.RealmName.ValueString() + "/smtp")
	m.Host = types.StringValue(c.Host)
	m.Port = types.Int64Value(c.Port)
	m.Encryption = types.StringValue(c.Encryption)
	m.FromEmail = types.StringValue(c.FromEmail)
	m.FromName = optionalString(c.FromName)
	m.AuthType = types.StringValue(c.AuthType)
	m.Username = optionalString(c.Username)
	m.HasPassword = types.BoolValue(c.HasPassword)
	m.TokenURL = optionalString(c.TokenURL)
	m.TokenScope = optionalString(c.TokenScope)
	m.ClientID = optionalString(c.ClientID)
	m.HasClientSecret = types.BoolValue(c.HasClientSecret)
	m.Status = types.StringValue(c.Status)
}
