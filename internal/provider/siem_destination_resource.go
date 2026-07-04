package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

var (
	_ resource.Resource                = (*siemDestinationResource)(nil)
	_ resource.ResourceWithConfigure   = (*siemDestinationResource)(nil)
	_ resource.ResourceWithImportState = (*siemDestinationResource)(nil)
)

type siemDestinationResource struct{ client *skycloak.Client }

// NewSIEMDestinationResource returns the skycloak_siem_destination resource.
func NewSIEMDestinationResource() resource.Resource { return &siemDestinationResource{} }

type siemSourceModel struct {
	Type               types.String   `tfsdk:"type"`
	ClusterIDs         []types.String `tfsdk:"cluster_ids"`
	Realms             []types.String `tfsdk:"realms"`
	KeycloakEventTypes []types.String `tfsdk:"keycloak_event_types"`
}

type siemBatchModel struct {
	MaxEvents          types.Int64 `tfsdk:"max_events"`
	MaxIntervalSeconds types.Int64 `tfsdk:"max_interval_seconds"`
}

type siemSyslogModel struct {
	Host     types.String `tfsdk:"host"`
	Port     types.Int64  `tfsdk:"port"`
	Protocol types.String `tfsdk:"protocol"`
	Format   types.String `tfsdk:"format"`
}

type siemS3Model struct {
	Bucket             types.String `tfsdk:"bucket"`
	Region             types.String `tfsdk:"region"`
	Prefix             types.String `tfsdk:"prefix"`
	AuthType           types.String `tfsdk:"auth_type"`
	AccessKeyID        types.String `tfsdk:"access_key_id"`
	SecretAccessKey    types.String `tfsdk:"secret_access_key"`
	RoleArn            types.String `tfsdk:"role_arn"`
	ExternalID         types.String `tfsdk:"external_id"`
	HasAccessKeySecret types.Bool   `tfsdk:"has_access_key_secret"`
}

type siemHTTPModel struct {
	URL                types.String            `tfsdk:"url"`
	AuthType           types.String            `tfsdk:"auth_type"`
	Username           types.String            `tfsdk:"username"`
	Password           types.String            `tfsdk:"password"`
	BearerToken        types.String            `tfsdk:"bearer_token"`
	Headers            map[string]types.String `tfsdk:"headers"`
	HasAuthCredentials types.Bool              `tfsdk:"has_auth_credentials"`
}

type siemDestinationModel struct {
	ID              types.String     `tfsdk:"id"`
	Name            types.String     `tfsdk:"name"`
	Enabled         types.Bool       `tfsdk:"enabled"`
	Type            types.String     `tfsdk:"type"`
	Source          *siemSourceModel `tfsdk:"source"`
	Batch           *siemBatchModel  `tfsdk:"batch"`
	Syslog          *siemSyslogModel `tfsdk:"syslog"`
	S3              *siemS3Model     `tfsdk:"s3"`
	HTTP            *siemHTTPModel   `tfsdk:"http"`
	HealthStatus    types.String     `tfsdk:"health_status"`
	FailureCount    types.Int64      `tfsdk:"failure_count"`
	LastError       types.String     `tfsdk:"last_error"`
	LastSentAt      types.String     `tfsdk:"last_sent_at"`
	TotalEventsSent types.Int64      `tfsdk:"total_events_sent"`
	TotalLogsSent   types.Int64      `tfsdk:"total_logs_sent"`
	TotalBytesSent  types.Int64      `tfsdk:"total_bytes_sent"`
	CreatedAt       types.String     `tfsdk:"created_at"`
	UpdatedAt       types.String     `tfsdk:"updated_at"`
}

func (r *siemDestinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_siem_destination"
}

func (r *siemDestinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A SIEM forwarding destination for the workspace (syslog, S3, or HTTP). " +
			"SIEM forwarding is plan-gated; the API rejects the calls on plans without it. " +
			"Secret fields are write-only: the API never returns them, reads report `has_*` booleans instead, " +
			"and Terraform keeps the configured values in state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Destination ID (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Destination name."},
			"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether forwarding is active. Defaults to `true`."},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Destination type: `syslog`, `s3`, or `http`. Immutable.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"source": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Which workspace data stream is forwarded.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{Required: true, MarkdownDescription: "Stream: `application_logs`, `keycloak_events`, `security_logs`, or `skycloak_audit`."},
					"cluster_ids": schema.ListAttribute{
						Optional: true, ElementType: types.StringType,
						MarkdownDescription: "Cluster IDs to include. Omit for all clusters.",
					},
					"realms": schema.ListAttribute{
						Optional: true, ElementType: types.StringType,
						MarkdownDescription: "Realm names to include. Omit for all realms.",
					},
					"keycloak_event_types": schema.ListAttribute{
						Optional: true, ElementType: types.StringType,
						MarkdownDescription: "Keycloak event type codes. Required when `type` is `keycloak_events`.",
					},
				},
			},
			"batch": schema.SingleNestedAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Batching tuning. Defaults to the API contract values " +
					"(1000 events, 60 seconds) so an omitted block plans exactly what the server applies.",
				Default: objectdefault.StaticValue(types.ObjectValueMust(
					map[string]attr.Type{"max_events": types.Int64Type, "max_interval_seconds": types.Int64Type},
					map[string]attr.Value{"max_events": types.Int64Value(1000), "max_interval_seconds": types.Int64Value(60)},
				)),
				Attributes: map[string]schema.Attribute{
					"max_events":           schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1000), MarkdownDescription: "Flush after this many events. Defaults to `1000`."},
					"max_interval_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(60), MarkdownDescription: "Flush at least this often. Defaults to `60`."},
				},
			},
			"syslog": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Syslog destination config (when `type` is `syslog`).",
				Attributes: map[string]schema.Attribute{
					"host":     schema.StringAttribute{Required: true, MarkdownDescription: "Syslog server hostname or IP."},
					"port":     schema.Int64Attribute{Required: true, MarkdownDescription: "Syslog server port."},
					"protocol": schema.StringAttribute{Required: true, MarkdownDescription: "`tcp`, `tls`, or `udp`."},
					"format":   schema.StringAttribute{Required: true, MarkdownDescription: "`rfc5424`, `cef`, `leef`, or `json`."},
				},
			},
			"s3": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "S3 destination config (when `type` is `s3`).",
				Attributes: map[string]schema.Attribute{
					"bucket":    schema.StringAttribute{Required: true, MarkdownDescription: "Bucket name."},
					"region":    schema.StringAttribute{Required: true, MarkdownDescription: "Bucket region."},
					"prefix":    schema.StringAttribute{Optional: true, MarkdownDescription: "Object key prefix."},
					"auth_type": schema.StringAttribute{Required: true, MarkdownDescription: "`access_key`, `assume_role`, `iam_role`, or `irsa`."},
					"access_key_id": schema.StringAttribute{
						Optional: true, Sensitive: true,
						MarkdownDescription: "Access key ID (when `auth_type` is `access_key`). Write-only.",
					},
					"secret_access_key": schema.StringAttribute{
						Optional: true, Sensitive: true,
						MarkdownDescription: "Secret access key (when `auth_type` is `access_key`). Write-only.",
					},
					"role_arn":              schema.StringAttribute{Optional: true, MarkdownDescription: "Role ARN (when `auth_type` is `assume_role`)."},
					"external_id":           schema.StringAttribute{Optional: true, MarkdownDescription: "External ID for role assumption."},
					"has_access_key_secret": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a secret access key is stored."},
				},
			},
			"http": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "HTTP destination config (when `type` is `http`).",
				Attributes: map[string]schema.Attribute{
					"url":       schema.StringAttribute{Required: true, MarkdownDescription: "Collector endpoint URL."},
					"auth_type": schema.StringAttribute{Required: true, MarkdownDescription: "`basic`, `bearer`, or `none`."},
					"username":  schema.StringAttribute{Optional: true, MarkdownDescription: "Username (when `auth_type` is `basic`)."},
					"password": schema.StringAttribute{
						Optional: true, Sensitive: true,
						MarkdownDescription: "Password (when `auth_type` is `basic`). Write-only.",
					},
					"bearer_token": schema.StringAttribute{
						Optional: true, Sensitive: true,
						MarkdownDescription: "Bearer token (when `auth_type` is `bearer`). Write-only.",
					},
					"headers": schema.MapAttribute{
						Optional: true, ElementType: types.StringType, Sensitive: true,
						MarkdownDescription: "Extra HTTP headers. Values are write-only; reads return header names only.",
					},
					"has_auth_credentials": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether auth credentials are stored."},
				},
			},
			"health_status":     schema.StringAttribute{Computed: true, MarkdownDescription: "`healthy`, `degraded`, or `failed`."},
			"failure_count":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Consecutive delivery failures."},
			"last_error":        schema.StringAttribute{Computed: true, MarkdownDescription: "Most recent delivery error."},
			"last_sent_at":      schema.StringAttribute{Computed: true, MarkdownDescription: "Last successful delivery."},
			"total_events_sent": schema.Int64Attribute{Computed: true},
			"total_logs_sent":   schema.Int64Attribute{Computed: true},
			"total_bytes_sent":  schema.Int64Attribute{Computed: true},
			"created_at":        schema.StringAttribute{Computed: true},
			"updated_at":        schema.StringAttribute{Computed: true},
		},
	}
}

func (r *siemDestinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func strList(in []types.String) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, v.ValueString())
	}
	return out
}

func (m *siemDestinationModel) toRequest() skycloak.CreateSIEMDestinationRequest {
	req := skycloak.CreateSIEMDestinationRequest{
		Name: m.Name.ValueString(),
		Type: m.Type.ValueString(),
	}
	enabled := m.Enabled.ValueBool()
	req.Enabled = &enabled
	if m.Source != nil {
		req.Source = skycloak.SIEMSource{
			Type:               m.Source.Type.ValueString(),
			ClusterIDs:         strList(m.Source.ClusterIDs),
			Realms:             strList(m.Source.Realms),
			KeycloakEventTypes: strList(m.Source.KeycloakEventTypes),
		}
	}
	if m.Batch != nil {
		req.Batch = &skycloak.SIEMBatch{
			MaxEvents:          m.Batch.MaxEvents.ValueInt64(),
			MaxIntervalSeconds: m.Batch.MaxIntervalSeconds.ValueInt64(),
		}
	}
	if m.Syslog != nil {
		req.Syslog = &skycloak.SIEMSyslog{
			Host: m.Syslog.Host.ValueString(), Port: m.Syslog.Port.ValueInt64(),
			Protocol: m.Syslog.Protocol.ValueString(), Format: m.Syslog.Format.ValueString(),
		}
	}
	if m.S3 != nil {
		req.S3 = &skycloak.SIEMS3{
			Bucket: m.S3.Bucket.ValueString(), Region: m.S3.Region.ValueString(),
			Prefix: m.S3.Prefix.ValueString(), AuthType: m.S3.AuthType.ValueString(),
			AccessKeyID: m.S3.AccessKeyID.ValueString(), SecretAccessKey: m.S3.SecretAccessKey.ValueString(),
			RoleArn: m.S3.RoleArn.ValueString(), ExternalID: m.S3.ExternalID.ValueString(),
		}
	}
	if m.HTTP != nil {
		headers := map[string]string{}
		for k, v := range m.HTTP.Headers {
			headers[k] = v.ValueString()
		}
		req.HTTP = &skycloak.SIEMHTTP{
			URL: m.HTTP.URL.ValueString(), AuthType: m.HTTP.AuthType.ValueString(),
			Username: m.HTTP.Username.ValueString(), Password: m.HTTP.Password.ValueString(),
			BearerToken: m.HTTP.BearerToken.ValueString(), Headers: headers,
		}
	}
	return req
}

// applyToModel maps API state onto the model, preserving write-only secret
// values already present in the model (the API never returns them).
func (m *siemDestinationModel) applyToModel(d *skycloak.SIEMDestination) {
	m.ID = types.StringValue(d.ID)
	m.Name = types.StringValue(d.Name)
	m.Enabled = types.BoolValue(d.Enabled)
	m.Type = types.StringValue(d.Type)

	src := &siemSourceModel{Type: types.StringValue(d.Source.Type)}
	for _, c := range d.Source.ClusterIDs {
		src.ClusterIDs = append(src.ClusterIDs, types.StringValue(c))
	}
	for _, rlm := range d.Source.Realms {
		src.Realms = append(src.Realms, types.StringValue(rlm))
	}
	for _, e := range d.Source.KeycloakEventTypes {
		src.KeycloakEventTypes = append(src.KeycloakEventTypes, types.StringValue(e))
	}
	m.Source = src

	if d.Batch != nil {
		m.Batch = &siemBatchModel{}
		if d.Batch.MaxEvents > 0 {
			m.Batch.MaxEvents = types.Int64Value(d.Batch.MaxEvents)
		} else {
			m.Batch.MaxEvents = types.Int64Null()
		}
		if d.Batch.MaxIntervalSeconds > 0 {
			m.Batch.MaxIntervalSeconds = types.Int64Value(d.Batch.MaxIntervalSeconds)
		} else {
			m.Batch.MaxIntervalSeconds = types.Int64Null()
		}
	}

	if d.Syslog != nil {
		m.Syslog = &siemSyslogModel{
			Host: types.StringValue(d.Syslog.Host), Port: types.Int64Value(d.Syslog.Port),
			Protocol: types.StringValue(d.Syslog.Protocol), Format: types.StringValue(d.Syslog.Format),
		}
	}

	if d.S3 != nil {
		prev := m.S3
		s3 := &siemS3Model{
			Bucket: types.StringValue(d.S3.Bucket), Region: types.StringValue(d.S3.Region),
			AuthType:           types.StringValue(d.S3.AuthType),
			Prefix:             optionalString(d.S3.Prefix),
			RoleArn:            optionalString(d.S3.RoleArn),
			ExternalID:         optionalString(d.S3.ExternalID),
			HasAccessKeySecret: types.BoolValue(d.S3.HasAccessKeySecret),
			AccessKeyID:        types.StringNull(),
			SecretAccessKey:    types.StringNull(),
		}
		if prev != nil {
			s3.AccessKeyID, s3.SecretAccessKey = prev.AccessKeyID, prev.SecretAccessKey
		}
		m.S3 = s3
	}

	if d.HTTP != nil {
		prev := m.HTTP
		h := &siemHTTPModel{
			URL: types.StringValue(d.HTTP.URL), AuthType: types.StringValue(d.HTTP.AuthType),
			HasAuthCredentials: types.BoolValue(d.HTTP.HasAuthCredentials),
			Username:           types.StringNull(),
			Password:           types.StringNull(),
			BearerToken:        types.StringNull(),
		}
		if prev != nil {
			h.Username, h.Password, h.BearerToken, h.Headers = prev.Username, prev.Password, prev.BearerToken, prev.Headers
		}
		m.HTTP = h
	}

	m.HealthStatus = types.StringValue(d.HealthStatus)
	m.FailureCount = types.Int64Value(d.FailureCount)
	m.LastError = optionalString(d.LastError)
	m.LastSentAt = optionalString(d.LastSentAt)
	m.TotalEventsSent = types.Int64Value(d.TotalEventsSent)
	m.TotalLogsSent = types.Int64Value(d.TotalLogsSent)
	m.TotalBytesSent = types.Int64Value(d.TotalBytesSent)
	m.CreatedAt = types.StringValue(d.CreatedAt)
	m.UpdatedAt = types.StringValue(d.UpdatedAt)
}

func (r *siemDestinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan siemDestinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.CreateSIEMDestination(ctx, plan.toRequest())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create SIEM destination", err.Error())
		return
	}
	plan.applyToModel(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siemDestinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state siemDestinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetSIEMDestination(ctx, state.ID.ValueString())
	if err != nil {
		if skycloak.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read SIEM destination", err.Error())
		return
	}
	state.applyToModel(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *siemDestinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan siemDestinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state siemDestinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.UpdateSIEMDestination(ctx, state.ID.ValueString(), plan.toRequest())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update SIEM destination", err.Error())
		return
	}
	plan.applyToModel(d)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siemDestinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state siemDestinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSIEMDestination(ctx, state.ID.ValueString()); err != nil && !skycloak.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete SIEM destination", err.Error())
	}
}

func (r *siemDestinationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
