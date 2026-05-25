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
	_ resource.Resource                = (*loginBrandingResource)(nil)
	_ resource.ResourceWithConfigure   = (*loginBrandingResource)(nil)
	_ resource.ResourceWithImportState = (*loginBrandingResource)(nil)
)

type loginBrandingResource struct {
	client *skycloak.Client
}

// NewLoginBrandingResource returns the skycloak_login_branding resource.
func NewLoginBrandingResource() resource.Resource { return &loginBrandingResource{} }

type loginI18nModel struct {
	Enabled                  types.Bool   `tfsdk:"enabled"`
	DefaultLocale            types.String `tfsdk:"default_locale"`
	SupportedLocales         types.List   `tfsdk:"supported_locales"`
	LanguageSelectionMode    types.String `tfsdk:"language_selection_mode"`
	LanguageSelectorPosition types.String `tfsdk:"language_selector_position"`
	LanguageSelectorStyle    types.String `tfsdk:"language_selector_style"`
}

type ssoModel struct {
	Enabled      types.Bool   `tfsdk:"enabled"`
	ButtonSize   types.String `tfsdk:"button_size"`
	DisplayStyle types.String `tfsdk:"display_style"`
	Layout       types.String `tfsdk:"layout"`
}

type loginBrandingModel struct {
	ID                    types.String    `tfsdk:"id"`
	ClusterID             types.String    `tfsdk:"cluster_id"`
	RealmName             types.String    `tfsdk:"realm_name"`
	PrimaryColor          types.String    `tfsdk:"primary_color"`
	BackgroundColor       types.String    `tfsdk:"background_color"`
	LogoURL               types.String    `tfsdk:"logo_url"`
	FaviconURL            types.String    `tfsdk:"favicon_url"`
	FontURL               types.String    `tfsdk:"font_url"`
	PrivacyPolicyURL      types.String    `tfsdk:"privacy_policy_url"`
	TermsOfServiceURL     types.String    `tfsdk:"terms_of_service_url"`
	ForgotPasswordEnabled types.Bool      `tfsdk:"forgot_password_enabled"`
	RegistrationEnabled   types.Bool      `tfsdk:"registration_enabled"`
	RememberMeEnabled     types.Bool      `tfsdk:"remember_me_enabled"`
	ShowPoweredBy         types.Bool      `tfsdk:"show_powered_by"`
	Internationalization  *loginI18nModel `tfsdk:"internationalization"`
	SSO                   *ssoModel       `tfsdk:"sso"`
	Status                types.String    `tfsdk:"status"`
}

func (r *loginBrandingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_login_branding"
}

func (r *loginBrandingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	toggle := schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Defaults to the API's value when unset."}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Login-page branding for a realm (a singleton; create == update upsert).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `cluster_id/realm_name/login_branding`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id":              schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: requiresReplace},
			"realm_name":              schema.StringAttribute{Required: true, MarkdownDescription: "Realm name. Immutable.", PlanModifiers: requiresReplace},
			"primary_color":           schema.StringAttribute{Optional: true, MarkdownDescription: "Primary accent color applied to buttons (hex, e.g. `#0ea5e9`)."},
			"background_color":        schema.StringAttribute{Optional: true, MarkdownDescription: "Login page background color (hex)."},
			"logo_url":                schema.StringAttribute{Optional: true, MarkdownDescription: "URL of the logo displayed on the login page."},
			"favicon_url":             schema.StringAttribute{Optional: true, MarkdownDescription: "URL of the browser favicon."},
			"font_url":                schema.StringAttribute{Optional: true, MarkdownDescription: "URL of a custom web font."},
			"privacy_policy_url":      schema.StringAttribute{Optional: true, MarkdownDescription: "URL of the privacy policy linked in the footer."},
			"terms_of_service_url":    schema.StringAttribute{Optional: true, MarkdownDescription: "URL of the terms of service linked in the footer."},
			"forgot_password_enabled": toggle,
			"registration_enabled":    toggle,
			"remember_me_enabled":     toggle,
			"show_powered_by":         toggle,
			"internationalization": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Login page internationalization settings.",
				Attributes: map[string]schema.Attribute{
					"enabled":                    schema.BoolAttribute{Required: true, MarkdownDescription: "Whether internationalization is enabled."},
					"default_locale":             schema.StringAttribute{Required: true, MarkdownDescription: "Default locale (BCP 47 tag, e.g. `en`)."},
					"supported_locales":          schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Locales available to users. Must include `default_locale`."},
					"language_selection_mode":    schema.StringAttribute{Required: true, MarkdownDescription: "`automatic_only`, `automatic_with_selector`, `hide_selector`, or `selector_only`."},
					"language_selector_position": schema.StringAttribute{Required: true, MarkdownDescription: "`form_inside_header`, `form_inside_left`, `top_left`, `top_middle`, or `top_right`."},
					"language_selector_style":    schema.StringAttribute{Required: true, MarkdownDescription: "`dropdown`, `flags`, or `text_labels`."},
				},
			},
			"sso": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Identity-provider button display on the login page.",
				Attributes: map[string]schema.Attribute{
					"enabled":       schema.BoolAttribute{Required: true, MarkdownDescription: "Show identity provider buttons on the login page."},
					"button_size":   schema.StringAttribute{Required: true, MarkdownDescription: "`small`, `medium`, or `large`."},
					"display_style": schema.StringAttribute{Required: true, MarkdownDescription: "`logo_only`, `logo_with_text`, or `text_only`."},
					"layout":        schema.StringAttribute{Required: true, MarkdownDescription: "`horizontal` or `vertical`."},
				},
			},
			"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Deployment status (`applied`, `applying`, `failed`)."},
		},
	}
}

func (r *loginBrandingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *loginBrandingResource) build(ctx context.Context, plan *loginBrandingModel, diags *diag.Diagnostics) skycloak.UpsertLoginBrandingRequest {
	req := skycloak.UpsertLoginBrandingRequest{
		PrimaryColor:          plan.PrimaryColor.ValueString(),
		BackgroundColor:       plan.BackgroundColor.ValueString(),
		LogoURL:               plan.LogoURL.ValueString(),
		FaviconURL:            plan.FaviconURL.ValueString(),
		FontURL:               plan.FontURL.ValueString(),
		PrivacyPolicyURL:      plan.PrivacyPolicyURL.ValueString(),
		TermsOfServiceURL:     plan.TermsOfServiceURL.ValueString(),
		ForgotPasswordEnabled: boolPtrIfKnown(plan.ForgotPasswordEnabled),
		RegistrationEnabled:   boolPtrIfKnown(plan.RegistrationEnabled),
		RememberMeEnabled:     boolPtrIfKnown(plan.RememberMeEnabled),
		ShowPoweredBy:         boolPtrIfKnown(plan.ShowPoweredBy),
	}
	if i := plan.Internationalization; i != nil {
		req.Internationalization = &skycloak.LoginI18n{
			Enabled:                  i.Enabled.ValueBool(),
			DefaultLocale:            i.DefaultLocale.ValueString(),
			SupportedLocales:         stringListToSlice(ctx, i.SupportedLocales, diags),
			LanguageSelectionMode:    i.LanguageSelectionMode.ValueString(),
			LanguageSelectorPosition: i.LanguageSelectorPosition.ValueString(),
			LanguageSelectorStyle:    i.LanguageSelectorStyle.ValueString(),
		}
	}
	if s := plan.SSO; s != nil {
		req.SSO = &skycloak.SSOConfig{
			Enabled: s.Enabled.ValueBool(), ButtonSize: s.ButtonSize.ValueString(),
			DisplayStyle: s.DisplayStyle.ValueString(), Layout: s.Layout.ValueString(),
		}
	}
	return req
}

func (r *loginBrandingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan loginBrandingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.build(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.UpsertLoginBranding(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to configure login branding", err.Error())
		return
	}
	applyLoginBrandingToModel(ctx, cfg, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *loginBrandingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state loginBrandingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.GetLoginBranding(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read login branding", err.Error())
		return
	}
	applyLoginBrandingToModel(ctx, cfg, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *loginBrandingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan loginBrandingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.build(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, err := r.client.UpsertLoginBranding(ctx, plan.ClusterID.ValueString(), plan.RealmName.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update login branding", err.Error())
		return
	}
	applyLoginBrandingToModel(ctx, cfg, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *loginBrandingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state loginBrandingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteLoginBranding(ctx, state.ClusterID.ValueString(), state.RealmName.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete login branding", err.Error())
	}
}

func (r *loginBrandingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/realm_name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("realm_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[0]+"/"+parts[1]+"/login_branding")...)
}

func applyLoginBrandingToModel(ctx context.Context, c *skycloak.LoginBranding, m *loginBrandingModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(m.ClusterID.ValueString() + "/" + m.RealmName.ValueString() + "/login_branding")
	m.PrimaryColor = optionalString(c.PrimaryColor)
	m.BackgroundColor = optionalString(c.BackgroundColor)
	m.LogoURL = optionalString(c.LogoURL)
	m.FaviconURL = optionalString(c.FaviconURL)
	m.FontURL = optionalString(c.FontURL)
	m.PrivacyPolicyURL = optionalString(c.PrivacyPolicyURL)
	m.TermsOfServiceURL = optionalString(c.TermsOfServiceURL)
	m.ForgotPasswordEnabled = types.BoolValue(c.ForgotPasswordEnabled)
	m.RegistrationEnabled = types.BoolValue(c.RegistrationEnabled)
	m.RememberMeEnabled = types.BoolValue(c.RememberMeEnabled)
	m.ShowPoweredBy = types.BoolValue(c.ShowPoweredBy)
	m.Status = types.StringValue(c.Status)
	if i := c.Internationalization; i != nil {
		m.Internationalization = &loginI18nModel{
			Enabled:                  types.BoolValue(i.Enabled),
			DefaultLocale:            types.StringValue(i.DefaultLocale),
			SupportedLocales:         sliceToStringList(ctx, i.SupportedLocales, diags),
			LanguageSelectionMode:    types.StringValue(i.LanguageSelectionMode),
			LanguageSelectorPosition: types.StringValue(i.LanguageSelectorPosition),
			LanguageSelectorStyle:    types.StringValue(i.LanguageSelectorStyle),
		}
	} else {
		m.Internationalization = nil
	}
	if s := c.SSO; s != nil {
		m.SSO = &ssoModel{
			Enabled: types.BoolValue(s.Enabled), ButtonSize: types.StringValue(s.ButtonSize),
			DisplayStyle: types.StringValue(s.DisplayStyle), Layout: types.StringValue(s.Layout),
		}
	} else {
		m.SSO = nil
	}
}

// boolPtrIfKnown returns a pointer to the bool when the value is set and known,
// or nil so the API applies its own default.
func boolPtrIfKnown(b types.Bool) *bool {
	if b.IsNull() || b.IsUnknown() {
		return nil
	}
	v := b.ValueBool()
	return &v
}
