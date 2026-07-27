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

// realmExportTimeout bounds the wait for a realm export job to finish.
const realmExportTimeout = 30 * time.Minute

var (
	_ resource.Resource                = (*realmExportResource)(nil)
	_ resource.ResourceWithConfigure   = (*realmExportResource)(nil)
	_ resource.ResourceWithImportState = (*realmExportResource)(nil)
)

type realmExportResource struct {
	client *skycloak.Client
}

// NewRealmExportResource returns the skycloak_realm_export resource.
func NewRealmExportResource() resource.Resource { return &realmExportResource{} }

type realmExportModel struct {
	ID                 types.String `tfsdk:"id"`
	ClusterID          types.String `tfsdk:"cluster_id"`
	Realm              types.String `tfsdk:"realm"`
	Scope              types.String `tfsdk:"scope"`
	EncryptionPassword types.String `tfsdk:"encryption_password"`
	Status             types.String `tfsdk:"status"`
	Progress           types.Int64  `tfsdk:"progress"`
	SourceVersion      types.String `tfsdk:"source_version"`
	Sha256Checksum     types.String `tfsdk:"sha256_checksum"`
	DownloadURL        types.String `tfsdk:"download_url"`
	CreatedAt          types.String `tfsdk:"created_at"`
	CompletedAt        types.String `tfsdk:"completed_at"`
	ExpiresAt          types.String `tfsdk:"expires_at"`
}

func (r *realmExportResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_realm_export"
}

func (r *realmExportResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A one-shot export of a single realm (configuration, users, and credentials). Creating the resource starts the export and waits for it to finish. " +
			"The archive is always encrypted and Skycloak deletes it 24 hours after completion, so `download_url` becomes null once `expires_at` has passed — recreate the resource to produce a fresh export.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Realm export job ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"realm":      schema.StringAttribute{Required: true, MarkdownDescription: "Name of the realm to export. Immutable.", PlanModifiers: rrStr},
			"scope": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("full"),
				MarkdownDescription: "Export scope. Only `full` is currently supported. Immutable.",
				PlanModifiers:       rrStr,
			},
			"encryption_password": schema.StringAttribute{
				Required: true, Sensitive: true,
				MarkdownDescription: "Password used to encrypt the archive (AES-256-CBC, PBKDF2), 8-128 characters. Required: every realm export contains credentials. " +
					"Write-only; never read back, so an imported resource plans a replacement until it is set. Immutable.",
				PlanModifiers: rrStr,
			},
			"status":          schema.StringAttribute{Computed: true, MarkdownDescription: "Job status (`pending`, `processing`, `completed`, `failed`)."},
			"progress":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Completion percentage (0-100)."},
			"source_version":  schema.StringAttribute{Computed: true, MarkdownDescription: "Keycloak version that produced the export. An import target must run this version or newer."},
			"sha256_checksum": schema.StringAttribute{Computed: true, MarkdownDescription: "SHA-256 checksum of the encrypted archive (hex)."},
			"download_url":    schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Time-limited presigned download URL. Null once the archive has expired."},
			"created_at":      schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp (RFC 3339)."},
			"completed_at":    schema.StringAttribute{Computed: true, MarkdownDescription: "Completion timestamp (RFC 3339)."},
			"expires_at":      schema.StringAttribute{Computed: true, MarkdownDescription: "When the archive is deleted from storage, 24h after completion (RFC 3339)."},
		},
	}
}

func (r *realmExportResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *realmExportResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan realmExportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	exp, err := r.client.CreateRealmExport(ctx, plan.ClusterID.ValueString(), plan.Realm.ValueString(), skycloak.CreateRealmExportRequest{
		Scope:              plan.Scope.ValueString(),
		EncryptionPassword: plan.EncryptionPassword.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to start realm export", err.Error())
		return
	}

	// The export is async (202). Poll until it finishes, bounded by realmExportTimeout.
	waitCtx, cancel := context.WithTimeout(ctx, realmExportTimeout)
	defer cancel()
	final, err := r.client.WaitForRealmExport(waitCtx, exp.ID)
	if err != nil {
		resp.Diagnostics.AddError("Realm export did not complete", err.Error())
		// Persist the ID so the job is tracked rather than leaked.
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), exp.ID)...)
		return
	}
	applyRealmExportToModel(final, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *realmExportResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state realmExportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	exp, err := r.client.GetRealmExport(ctx, state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read realm export", err.Error())
		return
	}
	applyRealmExportToModel(exp, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only runs for in-place changes. Every configurable attribute is
// RequiresReplace, so this just refreshes the job's current state.
func (r *realmExportResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan realmExportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	exp, err := r.client.GetRealmExport(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read realm export", err.Error())
		return
	}
	applyRealmExportToModel(exp, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete drops the job from state. The API exposes no delete for realm exports:
// the archive is retained for 24h and removed by Skycloak on expiry.
func (r *realmExportResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes the bare export ID: the export is addressed workspace-wide
// and its response carries the cluster and realm.
func (r *realmExportResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyRealmExportToModel copies API fields into the model. The write-only
// encryption_password already in the model is left untouched.
func applyRealmExportToModel(e *skycloak.RealmExport, m *realmExportModel) {
	m.ID = types.StringValue(e.ID)
	m.ClusterID = types.StringValue(e.ClusterID)
	m.Realm = types.StringValue(e.Realm)
	m.Scope = types.StringValue(e.Scope)
	m.Status = types.StringValue(e.Status)
	m.Progress = types.Int64Value(e.Progress)
	m.SourceVersion = optionalString(e.SourceVersion)
	m.Sha256Checksum = optionalString(e.Sha256Checksum)
	m.DownloadURL = optionalString(e.DownloadURL)
	m.CreatedAt = types.StringValue(e.CreatedAt)
	m.CompletedAt = optionalString(e.CompletedAt)
	m.ExpiresAt = optionalString(e.ExpiresAt)
}
