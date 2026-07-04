package provider

import (
	"context"
	"fmt"

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
	_ resource.Resource                = (*clusterSecurityResource)(nil)
	_ resource.ResourceWithConfigure   = (*clusterSecurityResource)(nil)
	_ resource.ResourceWithImportState = (*clusterSecurityResource)(nil)
)

type clusterSecurityResource struct {
	client *skycloak.Client
}

// NewClusterSecurityResource returns the skycloak_cluster_security resource.
func NewClusterSecurityResource() resource.Resource { return &clusterSecurityResource{} }

type ipPathRuleModel struct {
	Path         types.String `tfsdk:"path"`
	Description  types.String `tfsdk:"description"`
	AllowedIPs   types.List   `tfsdk:"allowed_ips"`
	AllowedCIDRs types.List   `tfsdk:"allowed_cidrs"`
}

type endpointLimitModel struct {
	Path types.String `tfsdk:"path"`
	RPM  types.Int64  `tfsdk:"rpm"`
}

type rateLimitingModel struct {
	Enabled        types.Bool           `tfsdk:"enabled"`
	GlobalRPM      types.Int64          `tfsdk:"global_rpm"`
	PerIPRPM       types.Int64          `tfsdk:"per_ip_rpm"`
	EndpointLimits []endpointLimitModel `tfsdk:"endpoint_limits"`
}

type wafCategoriesModel struct {
	CrossSiteScripting  types.Bool `tfsdk:"cross_site_scripting"`
	DataLeakage         types.Bool `tfsdk:"data_leakage"`
	JavaAttacks         types.Bool `tfsdk:"java_attacks"`
	LocalFileInclusion  types.Bool `tfsdk:"local_file_inclusion"`
	PhpInjection        types.Bool `tfsdk:"php_injection"`
	ProtocolAttacks     types.Bool `tfsdk:"protocol_attacks"`
	ProtocolEnforcement types.Bool `tfsdk:"protocol_enforcement"`
	RemoteCodeExecution types.Bool `tfsdk:"remote_code_execution"`
	RemoteFileInclusion types.Bool `tfsdk:"remote_file_inclusion"`
	SessionFixation     types.Bool `tfsdk:"session_fixation"`
	SQLInjection        types.Bool `tfsdk:"sql_injection"`
	WebshellDetection   types.Bool `tfsdk:"webshell_detection"`
}

type wafRuleExclusionModel struct {
	RuleIDs types.List `tfsdk:"rule_ids"`
	Paths   types.List `tfsdk:"paths"`
}

type wafModel struct {
	Enabled        types.Bool              `tfsdk:"enabled"`
	Mode           types.String            `tfsdk:"mode"`
	Preset         types.String            `tfsdk:"preset"`
	ParanoiaLevel  types.Int64             `tfsdk:"paranoia_level"`
	Categories     *wafCategoriesModel     `tfsdk:"categories"`
	ExclusionPaths types.List              `tfsdk:"exclusion_paths"`
	RuleExclusions []wafRuleExclusionModel `tfsdk:"rule_exclusions"`
}

type geoBlockingModel struct {
	Enabled   types.Bool   `tfsdk:"enabled"`
	Mode      types.String `tfsdk:"mode"`
	Countries types.List   `tfsdk:"countries"`
}

type botManagementModel struct {
	Enabled           types.Bool   `tfsdk:"enabled"`
	Mode              types.String `tfsdk:"mode"`
	ChallengeMode     types.String `tfsdk:"challenge_mode"`
	WhitelistedAgents types.List   `tfsdk:"whitelisted_agents"`
	BlacklistedAgents types.List   `tfsdk:"blacklisted_agents"`
}

type captchaModel struct {
	Enabled       types.Bool `tfsdk:"enabled"`
	EnabledRealms types.List `tfsdk:"enabled_realms"`
}

type clusterSecurityModel struct {
	ID              types.String        `tfsdk:"id"`
	ClusterID       types.String        `tfsdk:"cluster_id"`
	IPAccessControl []ipPathRuleModel   `tfsdk:"ip_access_control"`
	RateLimiting    *rateLimitingModel  `tfsdk:"rate_limiting"`
	WAF             *wafModel           `tfsdk:"waf"`
	GeoBlocking     *geoBlockingModel   `tfsdk:"geo_blocking"`
	BotManagement   *botManagementModel `tfsdk:"bot_management"`
	CAPTCHA         *captchaModel       `tfsdk:"captcha"`
}

func (r *clusterSecurityResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_security"
}

func (r *clusterSecurityResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Edge-security configuration for a cluster: IP allow-listing, rate limiting, WAF, geo-blocking, bot management, and CAPTCHA. A singleton per cluster. When the `captcha` block is omitted, the server's CAPTCHA settings are left untouched. CAPTCHA hostnames are registered via `skycloak_captcha_domain`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`cluster_id/security`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Cluster ID. Immutable.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ip_access_control": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Per-path IP allow rules.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path":          schema.StringAttribute{Required: true, MarkdownDescription: "URL path prefix to restrict."},
						"description":   schema.StringAttribute{Optional: true, MarkdownDescription: "Rule description."},
						"allowed_ips":   schema.ListAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "Individual IPs to allow."},
						"allowed_cidrs": schema.ListAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "CIDR ranges to allow."},
					},
				},
			},
			"rate_limiting": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Request-rate ceilings.",
				Attributes: map[string]schema.Attribute{
					"enabled":    schema.BoolAttribute{Required: true, MarkdownDescription: "Whether rate limiting is enabled."},
					"global_rpm": schema.Int64Attribute{Optional: true, MarkdownDescription: "Global requests-per-minute ceiling."},
					"per_ip_rpm": schema.Int64Attribute{Optional: true, MarkdownDescription: "Per-source-IP requests-per-minute ceiling."},
					"endpoint_limits": schema.ListNestedAttribute{
						Optional:            true,
						MarkdownDescription: "Per-path request limits.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"path": schema.StringAttribute{Required: true, MarkdownDescription: "URL path."},
								"rpm":  schema.Int64Attribute{Required: true, MarkdownDescription: "Max requests per minute."},
							},
						},
					},
				},
			},
			"waf": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Web application firewall.",
				Attributes: map[string]schema.Attribute{
					"enabled":        schema.BoolAttribute{Required: true, MarkdownDescription: "Whether the WAF is enabled."},
					"mode":           schema.StringAttribute{Required: true, MarkdownDescription: "`block` or `detect`."},
					"preset":         schema.StringAttribute{Required: true, MarkdownDescription: "`owasp_top_10`, `full_crs`, or `custom`."},
					"paranoia_level": schema.Int64Attribute{Required: true, MarkdownDescription: "Detection sensitivity (1-4)."},
					"categories": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: "Active rule categories (required when `preset` is `custom`).",
						Attributes: map[string]schema.Attribute{
							"cross_site_scripting":  schema.BoolAttribute{Required: true},
							"data_leakage":          schema.BoolAttribute{Required: true},
							"java_attacks":          schema.BoolAttribute{Required: true},
							"local_file_inclusion":  schema.BoolAttribute{Required: true},
							"php_injection":         schema.BoolAttribute{Required: true},
							"protocol_attacks":      schema.BoolAttribute{Required: true},
							"protocol_enforcement":  schema.BoolAttribute{Required: true},
							"remote_code_execution": schema.BoolAttribute{Required: true},
							"remote_file_inclusion": schema.BoolAttribute{Required: true},
							"session_fixation":      schema.BoolAttribute{Required: true},
							"sql_injection":         schema.BoolAttribute{Required: true},
							"webshell_detection":    schema.BoolAttribute{Required: true},
						},
					},
					"exclusion_paths": schema.ListAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "URL paths exempt from inspection."},
					"rule_exclusions": schema.ListNestedAttribute{
						Optional:            true,
						MarkdownDescription: "CRS rules to disable.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"rule_ids": schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "OWASP CRS rule IDs to disable."},
								"paths":    schema.ListAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "Restrict the exclusion to these paths."},
							},
						},
					},
				},
			},
			"geo_blocking": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Country-based access control.",
				Attributes: map[string]schema.Attribute{
					"enabled":   schema.BoolAttribute{Required: true, MarkdownDescription: "Whether geo-blocking is enabled."},
					"mode":      schema.StringAttribute{Required: true, MarkdownDescription: "`allowlist` or `blocklist`."},
					"countries": schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "ISO 3166-1 alpha-2 country codes."},
				},
			},
			"bot_management": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Bot detection and challenges.",
				Attributes: map[string]schema.Attribute{
					"enabled":            schema.BoolAttribute{Required: true, MarkdownDescription: "Whether bot management is enabled."},
					"mode":               schema.StringAttribute{Required: true, MarkdownDescription: "`block` or `detect`."},
					"challenge_mode":     schema.StringAttribute{Required: true, MarkdownDescription: "`none`, `javascript`, or `captcha`."},
					"whitelisted_agents": schema.ListAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "User-agent regex patterns to always allow."},
					"blacklisted_agents": schema.ListAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "User-agent regex patterns to always block."},
				},
			},
			"captcha": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Login-flow CAPTCHA challenges. Omit to leave the server's CAPTCHA settings unmanaged.",
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{Required: true, MarkdownDescription: "Whether CAPTCHA challenges are presented."},
					"enabled_realms": schema.ListAttribute{
						Optional: true, ElementType: types.StringType,
						MarkdownDescription: "Realm IDs where challenges are presented during login.",
					},
				},
			},
		},
	}
}

func (r *clusterSecurityResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *clusterSecurityResource) save(ctx context.Context, plan *clusterSecurityModel, diags *diag.Diagnostics) (*skycloak.ClusterSecurity, error) {
	return r.client.UpdateClusterSecurity(ctx, plan.ClusterID.ValueString(), securityFromModel(ctx, plan, diags))
}

func (r *clusterSecurityResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clusterSecurityModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sec, err := r.save(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to configure cluster security", err.Error())
		return
	}
	applySecurityToModel(ctx, sec, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clusterSecurityResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clusterSecurityModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sec, err := r.client.GetClusterSecurity(ctx, state.ClusterID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read cluster security", err.Error())
		return
	}
	applySecurityToModel(ctx, sec, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clusterSecurityResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clusterSecurityModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sec, err := r.save(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to update cluster security", err.Error())
		return
	}
	applySecurityToModel(ctx, sec, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete clears the managed sections (resets to an empty security config,
// preserving CAPTCHA).
func (r *clusterSecurityResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state clusterSecurityModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.UpdateClusterSecurity(ctx, state.ClusterID.ValueString(), &skycloak.ClusterSecurity{}); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to reset cluster security", err.Error())
	}
}

func (r *clusterSecurityResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID+"/security")...)
}
