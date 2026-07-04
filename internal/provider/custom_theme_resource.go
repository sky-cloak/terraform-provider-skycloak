package provider

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// themeDeployTimeout bounds the wait for an uploaded theme to finish
// deploying. Extraction and rollout of a theme package is far lighter than a
// cluster provision, so this is much shorter than createTimeout.
const themeDeployTimeout = 5 * time.Minute

var (
	_ resource.Resource                = (*customThemeResource)(nil)
	_ resource.ResourceWithConfigure   = (*customThemeResource)(nil)
	_ resource.ResourceWithImportState = (*customThemeResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*customThemeResource)(nil)
)

type customThemeResource struct {
	client *skycloak.Client
}

// NewCustomThemeResource returns the skycloak_custom_theme resource (custom theme upload).
func NewCustomThemeResource() resource.Resource { return &customThemeResource{} }

type customThemeResourceModel struct {
	ID            types.String `tfsdk:"id"`
	ClusterID     types.String `tfsdk:"cluster_id"`
	Source        types.String `tfsdk:"source"`
	ContentSHA256 types.String `tfsdk:"content_sha256"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	Version       types.String `tfsdk:"version"`
	ThemeTypes    types.List   `tfsdk:"theme_types"`
	Status        types.String `tfsdk:"status"`
	FileSize      types.Int64  `tfsdk:"file_size"`
	DeployedAt    types.String `tfsdk:"deployed_at"`
}

func (r *customThemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_theme"
}

func (r *customThemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Uploads a custom Keycloak theme (ZIP or Keycloakify JAR) to a cluster. Replacing the file contents or `theme_types` recreates the theme; `name`, `description`, and `version` update in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Theme ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"source":     schema.StringAttribute{Required: true, MarkdownDescription: "Path to the theme archive (`.zip` or `.jar`)."},
			"content_sha256": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA-256 of the uploaded file. Recomputed each plan; a change recreates the theme.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name":        schema.StringAttribute{Required: true, MarkdownDescription: "Theme name."},
			"description": schema.StringAttribute{Optional: true, MarkdownDescription: "Theme description."},
			"version":     schema.StringAttribute{Optional: true, MarkdownDescription: "Semantic version (e.g. `1.2.3`)."},
			"theme_types": schema.ListAttribute{
				Optional: true, ElementType: types.StringType,
				MarkdownDescription: "Subset of theme types to deploy (`login`, `account`, `admin`, `email`). Omit to deploy all detected types. Immutable.",
				PlanModifiers:       []planmodifier.List{listplanmodifier.RequiresReplace()},
			},
			"status":      schema.StringAttribute{Computed: true, MarkdownDescription: "Deployment status (`deployed`, `deploying`, `failed`, `undeploying`)."},
			"file_size":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the uploaded archive in bytes."},
			"deployed_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Last successful deployment timestamp (RFC 3339)."},
		},
	}
}

func (r *customThemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan hashes the local file so a change in its bytes is detected even
// when the path is unchanged.
func (r *customThemeResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy plan
	}
	var plan customThemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Source.IsUnknown() || plan.Source.IsNull() {
		return
	}
	hash, err := fileSHA256(plan.Source.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read theme file", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("content_sha256"), hash)...)
}

func (r *customThemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customThemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content, err := readFile(plan.Source.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read theme file", err.Error())
		return
	}
	theme, err := r.client.UploadTheme(ctx, plan.ClusterID.ValueString(), skycloak.UploadThemeRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Version:     plan.Version.ValueString(),
		ThemeTypes:  stringListToSlice(ctx, plan.ThemeTypes, &resp.Diagnostics),
		FileName:    filepath.Base(plan.Source.ValueString()),
		Content:     content,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to upload theme", err.Error())
		return
	}

	// Upload returns while Keycloak is still deploying the package; only a
	// deployed theme can be assigned to a realm or application client, so a
	// same-apply theme_assignment would otherwise race the async rollout.
	waitCtx, cancel := context.WithTimeout(ctx, themeDeployTimeout)
	defer cancel()
	deployed, err := r.client.WaitForThemeDeployed(waitCtx, plan.ClusterID.ValueString(), theme.ID)
	if err != nil {
		resp.Diagnostics.AddError("Theme did not finish deploying", err.Error())
		// Persist the ID so the half-deployed theme is tracked, not leaked.
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), theme.ID)...)
		return
	}

	applyThemeToModel(deployed, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customThemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customThemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	theme, err := r.client.GetTheme(ctx, state.ClusterID.ValueString(), state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read theme", err.Error())
		return
	}
	applyThemeToModel(theme, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *customThemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan customThemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	theme, err := r.client.UpdateThemeMetadata(ctx, plan.ClusterID.ValueString(), plan.ID.ValueString(),
		plan.Name.ValueString(), plan.Description.ValueString(), plan.Version.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update theme", err.Error())
		return
	}
	applyThemeToModel(theme, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customThemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customThemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteTheme(ctx, state.ClusterID.ValueString(), state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete theme", err.Error())
	}
}

func (r *customThemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/theme_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// applyThemeToModel copies API fields into the model, leaving the source path
// and content hash (managed from config/plan) untouched.
func applyThemeToModel(t *skycloak.Theme, m *customThemeResourceModel) {
	m.ID = types.StringValue(t.ID)
	m.Name = types.StringValue(t.Name)
	m.Description = optionalString(t.Description)
	m.Version = optionalString(t.Version)
	m.Status = types.StringValue(t.Status)
	m.FileSize = types.Int64Value(t.FileSize)
	m.DeployedAt = optionalString(t.DeployedAt)
}
