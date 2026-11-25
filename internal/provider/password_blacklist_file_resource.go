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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// passwordBlacklistWaitTimeout bounds the wait for an uploaded file to reach
// the cluster. It covers the restart that adds the blacklist folder when
// apply_now is set, so it is longer than a theme deploy.
const passwordBlacklistWaitTimeout = 15 * time.Minute

var (
	_ resource.Resource                = (*passwordBlacklistFileResource)(nil)
	_ resource.ResourceWithConfigure   = (*passwordBlacklistFileResource)(nil)
	_ resource.ResourceWithImportState = (*passwordBlacklistFileResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*passwordBlacklistFileResource)(nil)
)

type passwordBlacklistFileResource struct {
	client *skycloak.Client
}

// NewPasswordBlacklistFileResource returns the skycloak_password_blacklist_file resource.
func NewPasswordBlacklistFileResource() resource.Resource { return &passwordBlacklistFileResource{} }

type passwordBlacklistFileResourceModel struct {
	ID            types.String `tfsdk:"id"`
	ClusterID     types.String `tfsdk:"cluster_id"`
	Name          types.String `tfsdk:"name"`
	Source        types.String `tfsdk:"source"`
	ApplyNow      types.Bool   `tfsdk:"apply_now"`
	ContentSHA256 types.String `tfsdk:"content_sha256"`
	Size          types.Int64  `tfsdk:"size"`
	Status        types.String `tfsdk:"status"`
}

func (r *passwordBlacklistFileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_password_blacklist_file"
}

func (r *passwordBlacklistFileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rrStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Uploads a password blacklist file to a dedicated cluster, for Keycloak's `passwordBlacklist(<name>)` realm password policy. " +
			"Files cannot be changed after upload: new content needs a new `name`, and changing the name replaces the file. " +
			"The first file on a cluster adds a small persistent folder to the Keycloak pods, which takes a restart: it waits for the cluster's maintenance window unless `apply_now` is set. " +
			"A cluster holds at most 10 files of up to 25 MB each. Once the file is `ready`, set the realm password policy to use it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "File name, as used in `passwordBlacklist(<name>)`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID. Immutable.", PlanModifiers: rrStr},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the file on the cluster: letters, digits, dot, dash and underscore only, up to 255 characters, and unique on the cluster. A change replaces the file, which the API refuses while a realm password policy names the old file; create the new file first with `lifecycle { create_before_destroy = true }` and move the policy in between.",
				PlanModifiers:       rrStr,
			},
			"source": schema.StringAttribute{Required: true, MarkdownDescription: "Path to the file to upload, one password per line, up to 25 MB."},
			"apply_now": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Restart the cluster at once to add the blacklist folder instead of waiting for the maintenance window, and wait for the file to be `ready`. Defaults to `false`. It only matters for the first file on a cluster, or while an earlier file is still waiting for the restart. It is not stored by the API, so it reads as `false` after an import.",
			},
			"content_sha256": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA-256 of the file. Recomputed each plan; a change to the file's content under an unchanged `name` is refused.",
			},
			"size":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the file in bytes."},
			"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Delivery status (`waiting_for_restart`, `delivering`, `ready`, `failed`)."},
		},
	}
}

func (r *passwordBlacklistFileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan hashes the local file. A file on the cluster cannot be changed, so
// new bytes under the same name are refused here, at plan time, rather than
// failing the upload with a name conflict.
func (r *passwordBlacklistFileResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy plan
	}
	var plan passwordBlacklistFileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Source.IsUnknown() || plan.Source.IsNull() {
		return
	}
	hash, err := fileSHA256(plan.Source.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read password blacklist file", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("content_sha256"), hash)...)

	if req.State.Raw.IsNull() || plan.Name.IsUnknown() {
		return
	}
	var state passwordBlacklistFileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.Name.Equal(plan.Name) && state.ContentSHA256.ValueString() != hash {
		resp.Diagnostics.AddAttributeError(path.Root("source"), "Password blacklist files cannot be changed",
			fmt.Sprintf("The content of %q differs from the file already on the cluster. Files are immutable: set a new name to upload the new content.", plan.Name.ValueString()))
	}
}

func (r *passwordBlacklistFileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan passwordBlacklistFileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content, err := readFile(plan.Source.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read password blacklist file", err.Error())
		return
	}
	clusterID, name := plan.ClusterID.ValueString(), plan.Name.ValueString()
	file, err := r.client.UploadPasswordBlacklistFile(ctx, clusterID, skycloak.UploadPasswordBlacklistFileRequest{
		Name:     name,
		ApplyNow: plan.ApplyNow.ValueBool(),
		Content:  content,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to upload password blacklist file", err.Error())
		return
	}

	// Without apply_now the first file waits for the maintenance window, which
	// can be days away, so only a file that is already being delivered is
	// waited for. With apply_now the restart runs at once and the apply waits
	// until the file is usable by a realm policy.
	waitCtx, cancel := context.WithTimeout(ctx, passwordBlacklistWaitTimeout)
	defer cancel()
	ready, err := r.client.WaitForPasswordBlacklistFile(waitCtx, clusterID, name, plan.ApplyNow.ValueBool())
	if err != nil {
		// Track the uploaded file, so it is not leaked in the cluster.
		if ready == nil {
			ready = file
		}
		applyPasswordBlacklistFileToModel(ready, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError("Password blacklist file did not reach the cluster", err.Error())
		return
	}

	applyPasswordBlacklistFileToModel(ready, &plan)
	if plan.ContentSHA256.ValueString() != ready.SHA256 {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError("Password blacklist file does not match", "the API reports a different SHA-256 than the local file")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *passwordBlacklistFileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state passwordBlacklistFileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	file, err := r.client.GetPasswordBlacklistFile(ctx, state.ClusterID.ValueString(), state.Name.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read password blacklist file", err.Error())
		return
	}
	applyPasswordBlacklistFileToModel(file, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only records configuration the API does not own: the local source path
// and apply_now. Content and name changes never reach it, because the first is
// refused at plan time and the second replaces the file.
func (r *passwordBlacklistFileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan passwordBlacklistFileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *passwordBlacklistFileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state passwordBlacklistFileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePasswordBlacklistFile(ctx, state.ClusterID.ValueString(), state.Name.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete password blacklist file", err.Error())
	}
}

func (r *passwordBlacklistFileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected import ID in the form cluster_id/name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("apply_now"), false)...)
}

// applyPasswordBlacklistFileToModel copies API fields into the model, leaving
// the source path and apply_now (managed from config) untouched.
func applyPasswordBlacklistFileToModel(f *skycloak.PasswordBlacklistFile, m *passwordBlacklistFileResourceModel) {
	m.ID = types.StringValue(f.Name)
	m.Name = types.StringValue(f.Name)
	m.ContentSHA256 = types.StringValue(f.SHA256)
	m.Size = types.Int64Value(f.Size)
	m.Status = types.StringValue(f.Status)
}
