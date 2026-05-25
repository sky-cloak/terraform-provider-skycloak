package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// exportTimeout bounds the wait for an export job to finish.
const exportTimeout = 30 * time.Minute

var (
	_ resource.Resource                = (*exportResource)(nil)
	_ resource.ResourceWithConfigure   = (*exportResource)(nil)
	_ resource.ResourceWithImportState = (*exportResource)(nil)
)

type exportResource struct {
	client *skycloak.Client
}

// NewExportResource returns the skycloak_export resource.
func NewExportResource() resource.Resource { return &exportResource{} }

type exportModel struct {
	ID                 types.String `tfsdk:"id"`
	ClusterID          types.String `tfsdk:"cluster_id"`
	Format             types.String `tfsdk:"format"`
	IncludeCredentials types.Bool   `tfsdk:"include_credentials"`
	EncryptionPassword types.String `tfsdk:"encryption_password"`
	Status             types.String `tfsdk:"status"`
	Progress           types.Int64  `tfsdk:"progress"`
	IsEncrypted        types.Bool   `tfsdk:"is_encrypted"`
	FileSizeBytes      types.Int64  `tfsdk:"file_size_bytes"`
	Sha256Checksum     types.String `tfsdk:"sha256_checksum"`
	DownloadURL        types.String `tfsdk:"download_url"`
	CreatedAt          types.String `tfsdk:"created_at"`
	CompletedAt        types.String `tfsdk:"completed_at"`
	ExpiresAt          types.String `tfsdk:"expires_at"`
}

func (r *exportResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_export"
}

func (r *exportResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A one-shot database export job for a cluster. Creating the resource starts the export and waits for it to finish. The archive expires; recreate the resource to produce a fresh export.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Export job ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"format":     schema.StringAttribute{Required: true, MarkdownDescription: "Export format: `pgdump` or `sql`. Immutable.", PlanModifiers: rrStr},
			"include_credentials": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Include the Keycloak `credential` and related tables. Requires `encryption_password`. Immutable.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"encryption_password": schema.StringAttribute{
				Optional: true, Sensitive: true,
				MarkdownDescription: "Password used to encrypt the archive (AES-256-CBC). Required when `include_credentials` is true. Write-only; never read back. Immutable.",
				PlanModifiers:       rrStr,
			},
			"status":          schema.StringAttribute{Computed: true, MarkdownDescription: "Job status (`pending`, `processing`, `completed`, `failed`)."},
			"progress":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Completion percentage (0-100)."},
			"is_encrypted":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the archive is encrypted."},
			"file_size_bytes": schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the produced archive in bytes."},
			"sha256_checksum": schema.StringAttribute{Computed: true, MarkdownDescription: "SHA-256 checksum of the archive (hex)."},
			"download_url":    schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Time-limited presigned download URL. Expires; refresh to obtain a new one."},
			"created_at":      schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp (RFC 3339)."},
			"completed_at":    schema.StringAttribute{Computed: true, MarkdownDescription: "Completion timestamp (RFC 3339)."},
			"expires_at":      schema.StringAttribute{Computed: true, MarkdownDescription: "When the archive is deleted from storage (RFC 3339)."},
		},
	}
}

func (r *exportResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *exportResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan exportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	exp, err := r.client.CreateExport(ctx, plan.ClusterID.ValueString(), skycloak.CreateExportRequest{
		Format:             plan.Format.ValueString(),
		IncludeCredentials: plan.IncludeCredentials.ValueBool(),
		EncryptionPassword: plan.EncryptionPassword.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to start export", err.Error())
		return
	}

	// Export is async (202). Poll until it finishes, bounded by exportTimeout.
	waitCtx, cancel := context.WithTimeout(ctx, exportTimeout)
	defer cancel()
	final, err := r.client.WaitForExport(waitCtx, plan.ClusterID.ValueString(), exp.ID)
	if err != nil {
		resp.Diagnostics.AddError("Export did not complete", err.Error())
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), exp.ID)...)
		return
	}
	applyExportToModel(final, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *exportResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state exportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	exp, err := r.client.GetExport(ctx, state.ClusterID.ValueString(), state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read export", err.Error())
		return
	}
	applyExportToModel(exp, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only runs for in-place changes. All configurable attributes are
// RequiresReplace, so this re-reads current state.
func (r *exportResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan exportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	exp, err := r.client.GetExport(ctx, plan.ClusterID.ValueString(), plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read export", err.Error())
		return
	}
	applyExportToModel(exp, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *exportResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state exportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteExport(ctx, state.ClusterID.ValueString(), state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete export", err.Error())
	}
}

func (r *exportResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/export_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// applyExportToModel copies API fields into the model. The write-only
// encryption_password already in the model is left untouched.
func applyExportToModel(e *skycloak.Export, m *exportModel) {
	m.ID = types.StringValue(e.ID)
	m.Format = types.StringValue(e.Format)
	m.IncludeCredentials = types.BoolValue(e.IncludeCredentials)
	m.Status = types.StringValue(e.Status)
	m.Progress = types.Int64Value(e.Progress)
	m.IsEncrypted = types.BoolValue(e.IsEncrypted)
	m.FileSizeBytes = types.Int64Value(e.FileSizeBytes)
	m.Sha256Checksum = optionalString(e.Sha256Checksum)
	m.DownloadURL = optionalString(e.DownloadURL)
	m.CreatedAt = types.StringValue(e.CreatedAt)
	m.CompletedAt = optionalString(e.CompletedAt)
	m.ExpiresAt = optionalString(e.ExpiresAt)
}
