package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// realmImportTimeout bounds the wait for a realm import job to finish.
const realmImportTimeout = 30 * time.Minute

const (
	realmImportSourceUpload = "upload"
	realmImportSourceStored = "stored"
)

var (
	_ resource.Resource                   = (*realmImportResource)(nil)
	_ resource.ResourceWithConfigure      = (*realmImportResource)(nil)
	_ resource.ResourceWithImportState    = (*realmImportResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*realmImportResource)(nil)
	_ resource.ResourceWithValidateConfig = (*realmImportResource)(nil)
)

type realmImportResource struct {
	client *skycloak.Client
}

// NewRealmImportResource returns the skycloak_realm_import resource.
func NewRealmImportResource() resource.Resource { return &realmImportResource{} }

type realmImportModel struct {
	ID               types.String `tfsdk:"id"`
	ClusterID        types.String `tfsdk:"cluster_id"`
	SourceKind       types.String `tfsdk:"source_kind"`
	SourceFile       types.String `tfsdk:"source_file"`
	SourceFileSHA256 types.String `tfsdk:"source_file_sha256"`
	SourceExportID   types.String `tfsdk:"source_export_id"`
	Password         types.String `tfsdk:"password"`
	Realm            types.String `tfsdk:"realm"`
	Status           types.String `tfsdk:"status"`
	Progress         types.Int64  `tfsdk:"progress"`
	SourceVersion    types.String `tfsdk:"source_version"`
	TargetVersion    types.String `tfsdk:"target_version"`
	CreatedAt        types.String `tfsdk:"created_at"`
	CompletedAt      types.String `tfsdk:"completed_at"`
}

func (r *realmImportResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_import"
}

func (r *realmImportResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Imports a realm (configuration, users, and credentials) into a cluster. Creating the resource starts the import and waits for it to finish. " +
			"The resource tracks the import *job*, not the resulting realm: destroying it removes the job from state and leaves the imported realm in place. " +
			"Preflight validates the target Keycloak version, refuses a realm-name collision, and blocks on missing password-hash providers.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Realm import job ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Target cluster ID. Immutable.", PlanModifiers: rrStr},
			"source_kind": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString(realmImportSourceUpload),
				MarkdownDescription: "Where the artifact comes from: `upload` (default, uses `source_file`) or `stored` (uses `source_export_id`). Immutable.",
				PlanModifiers:       rrStr,
			},
			"source_file": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to a local realm artifact to upload. Required when `source_kind` is `upload`. Immutable.",
				PlanModifiers:       rrStr,
			},
			"source_file_sha256": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA-256 of `source_file`, tracked so a change to the file's bytes forces a new import even when the path is unchanged.",
				PlanModifiers:       rrStr,
			},
			"source_export_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ID of a stored `skycloak_realm_export` to import from. Required when `source_kind` is `stored`. Immutable.",
				PlanModifiers:       rrStr,
			},
			"password": schema.StringAttribute{
				Optional: true, Sensitive: true,
				MarkdownDescription: "Password that decrypts the artifact. Required for an encrypted archive (anything produced by `skycloak_realm_export`); omit only for a bare foreign realm JSON. " +
					"Write-only; never read back. Immutable.",
				PlanModifiers: rrStr,
			},
			"realm":          schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the imported realm, derived from the artifact."},
			"status":         schema.StringAttribute{Computed: true, MarkdownDescription: "Job status (`pending`, `processing`, `completed`, `failed`)."},
			"progress":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Completion percentage (0-100)."},
			"source_version": schema.StringAttribute{Computed: true, MarkdownDescription: "Keycloak version that produced the artifact."},
			"target_version": schema.StringAttribute{Computed: true, MarkdownDescription: "Target cluster's Keycloak version at import time."},
			"created_at":     schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp (RFC 3339)."},
			"completed_at":   schema.StringAttribute{Computed: true, MarkdownDescription: "Completion timestamp (RFC 3339)."},
		},
	}
}

func (r *realmImportResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ValidateConfig rejects source combinations the API would reject at apply time,
// so the mistake surfaces during plan.
func (r *realmImportResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg realmImportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// source_kind is Optional+Computed, so a null config value means the default.
	kind := realmImportSourceUpload
	if !cfg.SourceKind.IsNull() && !cfg.SourceKind.IsUnknown() {
		kind = cfg.SourceKind.ValueString()
	}
	switch kind {
	case realmImportSourceUpload:
		if cfg.SourceFile.IsNull() && !cfg.SourceFile.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root("source_file"), "Missing source_file",
				"source_kind is \"upload\", so source_file must point at a local realm artifact.")
		}
		if !cfg.SourceExportID.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("source_export_id"), "Unexpected source_export_id",
				"source_export_id applies only when source_kind is \"stored\".")
		}
	case realmImportSourceStored:
		if cfg.SourceExportID.IsNull() && !cfg.SourceExportID.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root("source_export_id"), "Missing source_export_id",
				"source_kind is \"stored\", so source_export_id must reference a realm export.")
		}
		if !cfg.SourceFile.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("source_file"), "Unexpected source_file",
				"source_file applies only when source_kind is \"upload\".")
		}
	default:
		resp.Diagnostics.AddAttributeError(path.Root("source_kind"), "Invalid source_kind",
			fmt.Sprintf("expected \"upload\" or \"stored\", got %q", kind))
	}
}

// ModifyPlan hashes the local artifact so a change in its bytes forces a new
// import even when the path is unchanged.
func (r *realmImportResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy plan
	}
	var plan realmImportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.SourceFile.IsUnknown() || plan.SourceFile.IsNull() {
		return
	}
	hash, err := fileSHA256(plan.SourceFile.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read realm artifact", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("source_file_sha256"), hash)...)
}

func (r *realmImportResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan realmImportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	clusterID := plan.ClusterID.ValueString()
	createReq := skycloak.CreateRealmImportRequest{
		SourceKind:     plan.SourceKind.ValueString(),
		SourceExportID: plan.SourceExportID.ValueString(),
		Password:       plan.Password.ValueString(),
	}

	// An uploaded artifact is staged in object storage first: ask for a presigned
	// PUT, upload the bytes, then hand the returned key to the import.
	if createReq.SourceKind == realmImportSourceUpload {
		content, err := readFile(plan.SourceFile.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read realm artifact", err.Error())
			return
		}
		presigned, err := r.client.PresignRealmImportUpload(ctx, clusterID)
		if err != nil {
			resp.Diagnostics.AddError("Unable to get an upload URL", err.Error())
			return
		}
		if err := r.client.UploadRealmImportArtifact(ctx, presigned.UploadURL, content); err != nil {
			resp.Diagnostics.AddError("Unable to upload the realm artifact", err.Error())
			return
		}
		createReq.UploadS3Key = presigned.S3Key
	}

	imp, err := r.client.CreateRealmImport(ctx, clusterID, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Unable to start realm import", err.Error())
		return
	}

	// The import is async (202). Poll until it finishes, bounded by realmImportTimeout.
	waitCtx, cancel := context.WithTimeout(ctx, realmImportTimeout)
	defer cancel()
	final, err := r.client.WaitForRealmImport(waitCtx, imp.ID)
	if err != nil {
		resp.Diagnostics.AddError("Realm import did not complete", err.Error())
		// Persist the ID so a partially-applied import is tracked, not leaked.
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), imp.ID)...)
		return
	}
	applyRealmImportToModel(final, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmImportResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmImportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	imp, err := r.client.GetRealmImport(ctx, state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read realm import", err.Error())
		return
	}
	applyRealmImportToModel(imp, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only runs for in-place changes. Every configurable attribute is
// RequiresReplace, so this just refreshes the job's current state.
func (r *realmImportResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan realmImportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	imp, err := r.client.GetRealmImport(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read realm import", err.Error())
		return
	}
	applyRealmImportToModel(imp, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete drops the job from state. The API exposes no delete for realm imports,
// and the realm the import created is intentionally left running — remove it
// with skycloak_realm if that is what you want.
func (r *realmImportResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes the bare import ID: the job is addressed workspace-wide and
// its response carries the cluster.
func (r *realmImportResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyRealmImportToModel copies API fields into the model. The write-only
// password and the local source_file/source_file_sha256 are left untouched.
func applyRealmImportToModel(i *skycloak.RealmImport, m *realmImportModel) {
	m.ID = types.StringValue(i.ID)
	m.ClusterID = types.StringValue(i.ClusterID)
	m.SourceKind = types.StringValue(i.SourceKind)
	m.Realm = types.StringValue(i.Realm)
	m.Status = types.StringValue(i.Status)
	m.Progress = types.Int64Value(i.Progress)
	m.SourceVersion = optionalString(i.SourceVersion)
	m.TargetVersion = optionalString(i.TargetVersion)
	m.CreatedAt = types.StringValue(i.CreatedAt)
	m.CompletedAt = optionalString(i.CompletedAt)
}
