package provider

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*customExtensionResource)(nil)
	_ resource.ResourceWithConfigure   = (*customExtensionResource)(nil)
	_ resource.ResourceWithImportState = (*customExtensionResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*customExtensionResource)(nil)
)

type customExtensionResource struct {
	client *skycloak.Client
}

// NewCustomExtensionResource returns the skycloak_custom_extension resource.
func NewCustomExtensionResource() resource.Resource { return &customExtensionResource{} }

type paramOptionModel struct {
	Label types.String `tfsdk:"label"`
	Value types.String `tfsdk:"value"`
}

type paramModel struct {
	Key          types.String       `tfsdk:"key"`
	Label        types.String       `tfsdk:"label"`
	Type         types.String       `tfsdk:"type"`
	Required     types.Bool         `tfsdk:"required"`
	DefaultValue types.String       `tfsdk:"default_value"`
	IsSensitive  types.Bool         `tfsdk:"is_sensitive"`
	Options      []paramOptionModel `tfsdk:"options"`
}

type customExtensionModel struct {
	ID              types.String `tfsdk:"id"`
	Jar             types.String `tfsdk:"jar"`
	ContentSHA256   types.String `tfsdk:"content_sha256"`
	Name            types.String `tfsdk:"name"`
	KeycloakVersion types.String `tfsdk:"keycloak_version"`
	Version         types.String `tfsdk:"version"`
	Description     types.String `tfsdk:"description"`
	IconURL         types.String `tfsdk:"icon_url"`
	RepositoryURL   types.String `tfsdk:"repository_url"`
	ParameterType   types.String `tfsdk:"parameter_type"`
	Parameters      []paramModel `tfsdk:"parameters"`
	Source          types.String `tfsdk:"source"`
	ScanStatus      types.String `tfsdk:"scan_status"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

func (r *customExtensionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_extension"
}

func (r *customExtensionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Uploads a custom Keycloak extension JAR to the workspace catalog. Changing the JAR contents or `version` publishes a new version; other metadata updates in place. Install it on a cluster with `skycloak_cluster_extension`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Extension ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"jar": schema.StringAttribute{Required: true, MarkdownDescription: "Path to the extension JAR file."},
			"content_sha256": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA-256 of the uploaded JAR. Recomputed each plan; a change publishes a new version.",
			},
			"name":             schema.StringAttribute{Required: true, MarkdownDescription: "Extension name."},
			"keycloak_version": schema.StringAttribute{Required: true, MarkdownDescription: "Keycloak major version this JAR targets (e.g. `26`)."},
			"version":          schema.StringAttribute{Required: true, MarkdownDescription: "Semantic version (e.g. `1.0.0`). Bump it together with the JAR to publish a new version."},
			"description":      schema.StringAttribute{Optional: true, MarkdownDescription: "Extension description."},
			"icon_url":         schema.StringAttribute{Optional: true, MarkdownDescription: "Icon URL."},
			"repository_url":   schema.StringAttribute{Optional: true, MarkdownDescription: "Source repository URL."},
			"parameter_type":   schema.StringAttribute{Optional: true, MarkdownDescription: "How parameters are supplied to the extension."},
			"parameters": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Configuration parameter schema exposed when installing the extension.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key":           schema.StringAttribute{Required: true, MarkdownDescription: "Unique parameter identifier."},
						"label":         schema.StringAttribute{Required: true, MarkdownDescription: "Human-readable label."},
						"type":          schema.StringAttribute{Required: true, MarkdownDescription: "Field type: `text`, `password`, `number`, `checkbox`, or `dropdown`."},
						"required":      schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether the parameter must be supplied."},
						"default_value": schema.StringAttribute{Optional: true, MarkdownDescription: "Default value."},
						"is_sensitive":  schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether the value is write-only/secret."},
						"options": schema.ListNestedAttribute{
							Optional:            true,
							MarkdownDescription: "Choices for a `dropdown` parameter.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"label": schema.StringAttribute{Required: true, MarkdownDescription: "Option label."},
									"value": schema.StringAttribute{Required: true, MarkdownDescription: "Option value."},
								},
							},
						},
					},
				},
			},
			"source":      schema.StringAttribute{Computed: true, MarkdownDescription: "Extension source (always `custom` for this resource)."},
			"scan_status": schema.StringAttribute{Computed: true, MarkdownDescription: "Malware scan result."},
			"created_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp (RFC 3339)."},
			"updated_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Last update timestamp (RFC 3339)."},
		},
	}
}

func (r *customExtensionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *customExtensionResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan customExtensionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Jar.IsUnknown() || plan.Jar.IsNull() {
		return
	}
	hash, err := fileSHA256(plan.Jar.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read extension JAR", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("content_sha256"), hash)...)
}

// uploadRequest builds the facade request from the plan (excluding the JAR bytes).
func (m *customExtensionModel) uploadRequest() skycloak.UploadExtensionRequest {
	return skycloak.UploadExtensionRequest{
		Name:            m.Name.ValueString(),
		KeycloakVersion: m.KeycloakVersion.ValueString(),
		Description:     m.Description.ValueString(),
		IconURL:         m.IconURL.ValueString(),
		RepositoryURL:   m.RepositoryURL.ValueString(),
		Version:         m.Version.ValueString(),
		ParameterType:   m.ParameterType.ValueString(),
		Parameters:      toFacadeParams(m.Parameters),
	}
}

func toFacadeParams(in []paramModel) []skycloak.ExtensionParameterDef {
	out := make([]skycloak.ExtensionParameterDef, 0, len(in))
	for _, p := range in {
		def := skycloak.ExtensionParameterDef{
			Key: p.Key.ValueString(), Label: p.Label.ValueString(), Type: p.Type.ValueString(),
			Required: p.Required.ValueBool(), DefaultValue: p.DefaultValue.ValueString(), IsSensitive: p.IsSensitive.ValueBool(),
		}
		for _, o := range p.Options {
			def.Options = append(def.Options, skycloak.ExtensionParameterOption{Label: o.Label.ValueString(), Value: o.Value.ValueString()})
		}
		out = append(out, def)
	}
	return out
}

func (r *customExtensionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customExtensionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	jar, err := readFile(plan.Jar.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read extension JAR", err.Error())
		return
	}
	body := plan.uploadRequest()
	body.JarFileName = filepath.Base(plan.Jar.ValueString())
	body.Jar = jar
	ext, err := r.client.UploadExtension(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to upload extension", err.Error())
		return
	}
	applyExtensionToModel(ext, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customExtensionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customExtensionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ext, err := r.client.GetExtension(ctx, state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read extension", err.Error())
		return
	}
	applyExtensionToModel(ext, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *customExtensionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state customExtensionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A changed JAR or version publishes a new version.
	if plan.ContentSHA256.ValueString() != state.ContentSHA256.ValueString() || plan.Version.ValueString() != state.Version.ValueString() {
		jar, err := readFile(plan.Jar.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read extension JAR", err.Error())
			return
		}
		if _, err := r.client.PublishExtensionVersion(ctx, plan.ID.ValueString(), plan.Version.ValueString(), filepath.Base(plan.Jar.ValueString()), jar); err != nil {
			resp.Diagnostics.AddError("Unable to publish extension version", err.Error())
			return
		}
	}

	ext, err := r.client.UpdateExtensionMetadata(ctx, plan.ID.ValueString(), plan.uploadRequest())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update extension", err.Error())
		return
	}
	applyExtensionToModel(ext, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *customExtensionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customExtensionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteExtension(ctx, state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete extension", err.Error())
	}
}

func (r *customExtensionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyExtensionToModel copies returned metadata into the model. The JAR path,
// content hash, version, keycloak_version, and parameters are managed from
// config and left untouched.
func applyExtensionToModel(e *skycloak.ExtensionInfo, m *customExtensionModel) {
	m.ID = types.StringValue(e.ID)
	m.Name = types.StringValue(e.Name)
	m.Description = optionalString(e.Description)
	m.IconURL = optionalString(e.IconURL)
	m.RepositoryURL = optionalString(e.RepositoryURL)
	m.ParameterType = optionalString(e.ParameterType)
	m.Source = types.StringValue(e.Source)
	m.ScanStatus = optionalString(e.ScanStatus)
	m.CreatedAt = types.StringValue(e.CreatedAt)
	m.UpdatedAt = types.StringValue(e.UpdatedAt)
}
