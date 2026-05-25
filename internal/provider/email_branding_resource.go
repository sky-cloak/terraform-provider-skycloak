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
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*emailBrandingResource)(nil)
	_ resource.ResourceWithConfigure   = (*emailBrandingResource)(nil)
	_ resource.ResourceWithImportState = (*emailBrandingResource)(nil)
)

type emailBrandingResource struct {
	client *skycloak.Client
}

// NewEmailBrandingResource returns the skycloak_email_branding resource.
func NewEmailBrandingResource() resource.Resource { return &emailBrandingResource{} }

type i18nModel struct {
	Enabled          types.Bool   `tfsdk:"enabled"`
	DefaultLocale    types.String `tfsdk:"default_locale"`
	SupportedLocales types.List   `tfsdk:"supported_locales"`
}

type emailBrandingModel struct {
	ID                   types.String `tfsdk:"id"`
	ClusterID            types.String `tfsdk:"cluster_id"`
	RealmName            types.String `tfsdk:"realm_name"`
	PrimaryColor         types.String `tfsdk:"primary_color"`
	HeaderLogoLight      types.String `tfsdk:"header_logo_light_url"`
	HeaderLogoDark       types.String `tfsdk:"header_logo_dark_url"`
	FooterText           types.String `tfsdk:"footer_text"`
	FooterCompanyName    types.String `tfsdk:"footer_company_name"`
	CompanyURL           types.String `tfsdk:"company_url"`
	Internationalization *i18nModel   `tfsdk:"internationalization"`
	Status               types.String `tfsdk:"status"`
}

func (r *emailBrandingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_email_branding"
}

func (r *emailBrandingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Email-template branding for a realm (a singleton; create == update upsert).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/realm_name/email_branding`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id":            schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: requiresReplace},
			"realm_name":            schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: requiresReplace},
			"primary_color":         schema.StringAttribute{Optional: true, MarkdownDescription: "Primary accent color (hex, e.g. `#0ea5e9`)."},
			"header_logo_light_url": schema.StringAttribute{Optional: true, MarkdownDescription: "Logo URL for light-background email clients."},
			"header_logo_dark_url":  schema.StringAttribute{Optional: true, MarkdownDescription: "Logo URL for dark-background email clients."},
			"footer_text":           schema.StringAttribute{Optional: true, MarkdownDescription: "Free-text shown in the email footer."},
			"footer_company_name":   schema.StringAttribute{Optional: true, MarkdownDescription: "Company name shown in the email footer."},
			"company_url":           schema.StringAttribute{Optional: true, MarkdownDescription: "URL linked from the email footer."},
			"internationalization":  emailI18nSchema(),
			"status":                schema.StringAttribute{Computed: true, MarkdownDescription: "Deployment status (`applied`, `applying`, `failed`)."},
		},
	}
}

func emailI18nSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Email internationalization settings.",
		Attributes: map[string]schema.Attribute{
			"enabled":           schema.BoolAttribute{Required: true, MarkdownDescription: "Whether internationalization is enabled."},
			"default_locale":    schema.StringAttribute{Required: true, MarkdownDescription: "Default locale (BCP 47 tag, e.g. `en`)."},
			"supported_locales": schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Locales available to users. Must include `default_locale`."},
		},
	}
}

func (r *emailBrandingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *emailBrandingResource) build(ctx context.Context, plan *emailBrandingModel, diags *diag.Diagnostics) skycloak.UpsertEmailBrandingRequest {
	req := skycloak.UpsertEmailBrandingRequest{
		PrimaryColor:       plan.PrimaryColor.ValueString(),
		HeaderLogoLightURL: plan.HeaderLogoLight.ValueString(),
		HeaderLogoDarkURL:  plan.HeaderLogoDark.ValueString(),
		FooterText:         plan.FooterText.ValueString(),
		FooterCompanyName:  plan.FooterCompanyName.ValueString(),
		CompanyURL:         plan.CompanyURL.ValueString(),
	}
	if i := plan.Internationalization; i != nil {
		req.Internationalization = &skycloak.EmailI18n{
			Enabled:          i.Enabled.ValueBool(),
			DefaultLocale:    i.DefaultLocale.ValueString(),
			SupportedLocales: stringListToSlice(ctx, i.SupportedLocales, diags),
		}
	}
	return req
}

func (r *emailBrandingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan emailBrandingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.build(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.UpsertEmailBranding(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to configure email branding", err.Error())
		return
	}
	applyEmailBrandingToModel(ctx, cfg, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailBrandingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state emailBrandingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.GetEmailBranding(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read email branding", err.Error())
		return
	}
	applyEmailBrandingToModel(ctx, cfg, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailBrandingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan emailBrandingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.build(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.UpsertEmailBranding(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update email branding", err.Error())
		return
	}
	applyEmailBrandingToModel(ctx, cfg, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailBrandingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state emailBrandingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteEmailBranding(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete email branding", err.Error())
	}
}

func (r *emailBrandingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[0]+"/"+parts[1]+"/email_branding")...)
}

func applyEmailBrandingToModel(ctx context.Context, c *skycloak.EmailBranding, m *emailBrandingModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/" + m.RealmName.ValueString() + "/email_branding")
	m.PrimaryColor = optionalString(c.PrimaryColor)
	m.HeaderLogoLight = optionalString(c.HeaderLogoLightURL)
	m.HeaderLogoDark = optionalString(c.HeaderLogoDarkURL)
	m.FooterText = optionalString(c.FooterText)
	m.FooterCompanyName = optionalString(c.FooterCompanyName)
	m.CompanyURL = optionalString(c.CompanyURL)
	m.Status = types.StringValue(c.Status)
	if c.Internationalization != nil {
		m.Internationalization = &i18nModel{
			Enabled:          types.BoolValue(c.Internationalization.Enabled),
			DefaultLocale:    types.StringValue(c.Internationalization.DefaultLocale),
			SupportedLocales: sliceToStringList(ctx, c.Internationalization.SupportedLocales, diags),
		}
	} else {
		m.Internationalization = nil
	}
}
