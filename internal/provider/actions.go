package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/skycloak"
)

// Actions are imperative, one-shot operations (Terraform >= 1.14). They map
// the public API's verification and cancellation endpoints, which have no
// declarative lifecycle.

// actionClient handles the shared Configure plumbing for all actions.
type actionClient struct{ client *skycloak.Client }

func (a *actionClient) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*skycloak.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *skycloak.Client, got %T", req.ProviderData))
		return
	}
	a.client = client
}

// ---- skycloak_test_smtp ----

var (
	_ action.Action              = (*testSMTPAction)(nil)
	_ action.ActionWithConfigure = (*testSMTPAction)(nil)
)

type testSMTPAction struct{ actionClient }

// NewTestSMTPAction returns the skycloak_test_smtp action.
func NewTestSMTPAction() action.Action { return &testSMTPAction{} }

type testSMTPModel struct {
	ClusterID types.String `tfsdk:"cluster_id"`
	RealmName types.String `tfsdk:"realm_name"`
	Email     types.String `tfsdk:"email"`
}

func (a *testSMTPAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_smtp"
}

func (a *testSMTPAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sends a probe email through a realm's saved SMTP configuration and fails if delivery fails.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"realm_name": schema.StringAttribute{Required: true, MarkdownDescription: "Realm name."},
			"email":      schema.StringAttribute{Required: true, MarkdownDescription: "Recipient address for the test message."},
		},
	}
}

func (a *testSMTPAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg testSMTPModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	res, err := a.client.TestSMTP(ctx, cfg.ClusterID.ValueString(), cfg.RealmName.ValueString(), cfg.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("SMTP test failed to run", err.Error())
		return
	}
	if !res.Success {
		resp.Diagnostics.AddError("SMTP test failed", res.Message)
		return
	}
	resp.SendProgress(action.InvokeProgressEvent{Message: "SMTP test succeeded: " + res.Message})
}

// ---- skycloak_test_identity_provider ----

var (
	_ action.Action              = (*testIdentityProviderAction)(nil)
	_ action.ActionWithConfigure = (*testIdentityProviderAction)(nil)
)

type testIdentityProviderAction struct{ actionClient }

// NewTestIdentityProviderAction returns the skycloak_test_identity_provider action.
func NewTestIdentityProviderAction() action.Action { return &testIdentityProviderAction{} }

type testIdentityProviderModel struct {
	ClusterID    types.String `tfsdk:"cluster_id"`
	RealmName    types.String `tfsdk:"realm_name"`
	ProviderID   types.String `tfsdk:"provider_id"`
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
}

func (a *testIdentityProviderAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_identity_provider"
}

func (a *testIdentityProviderAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Validates an identity provider's connection (discovery, credentials) and fails on an unreachable or misconfigured provider.",
		Attributes: map[string]schema.Attribute{
			"cluster_id":  schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
			"realm_name":  schema.StringAttribute{Required: true, MarkdownDescription: "Realm name."},
			"provider_id": schema.StringAttribute{Required: true, MarkdownDescription: "Identity provider alias."},
			"client_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Override client ID for this test only. Not persisted.",
			},
			"client_secret": schema.StringAttribute{
				Optional:            true,
				WriteOnly:           true,
				MarkdownDescription: "Override client secret or LDAP bind password for this test only. Not persisted.",
			},
		},
	}
}

func (a *testIdentityProviderAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg testIdentityProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	res, err := a.client.TestIdentityProvider(ctx, cfg.ClusterID.ValueString(), cfg.RealmName.ValueString(),
		cfg.ProviderID.ValueString(), cfg.ClientID.ValueString(), cfg.ClientSecret.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Identity provider test failed to run", err.Error())
		return
	}
	if !res.Success {
		detail := res.Message
		for check, msg := range res.Details {
			detail += fmt.Sprintf("\n%s: %s", check, msg)
		}
		resp.Diagnostics.AddError("Identity provider test failed", detail)
		return
	}
	resp.SendProgress(action.InvokeProgressEvent{Message: "Identity provider test succeeded: " + res.Message})
}

// ---- skycloak_cancel_cluster_upgrade ----

var (
	_ action.Action              = (*cancelClusterUpgradeAction)(nil)
	_ action.ActionWithConfigure = (*cancelClusterUpgradeAction)(nil)
)

type cancelClusterUpgradeAction struct{ actionClient }

// NewCancelClusterUpgradeAction returns the skycloak_cancel_cluster_upgrade action.
func NewCancelClusterUpgradeAction() action.Action { return &cancelClusterUpgradeAction{} }

type cancelClusterUpgradeModel struct {
	ClusterID types.String `tfsdk:"cluster_id"`
}

func (a *cancelClusterUpgradeAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cancel_cluster_upgrade"
}

func (a *cancelClusterUpgradeAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Cancels a scheduled or in-progress Keycloak upgrade on a cluster. Fails when no upgrade is cancellable.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Required: true, MarkdownDescription: "Cluster ID."},
		},
	}
}

func (a *cancelClusterUpgradeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg cancelClusterUpgradeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cl, err := a.client.CancelClusterUpgrade(ctx, cfg.ClusterID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to cancel cluster upgrade", err.Error())
		return
	}
	resp.SendProgress(action.InvokeProgressEvent{Message: "Upgrade cancelled; cluster status: " + cl.Status})
}

// ---- skycloak_test_siem_destination ----

var (
	_ action.Action              = (*testSIEMDestinationAction)(nil)
	_ action.ActionWithConfigure = (*testSIEMDestinationAction)(nil)
)

type testSIEMDestinationAction struct{ actionClient }

// NewTestSIEMDestinationAction returns the skycloak_test_siem_destination action.
func NewTestSIEMDestinationAction() action.Action { return &testSIEMDestinationAction{} }

type testSIEMDestinationModel struct {
	DestinationID types.String `tfsdk:"destination_id"`
}

func (a *testSIEMDestinationAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_siem_destination"
}

func (a *testSIEMDestinationAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sends a synthetic event through a saved SIEM destination and fails if delivery fails.",
		Attributes: map[string]schema.Attribute{
			"destination_id": schema.StringAttribute{Required: true, MarkdownDescription: "SIEM destination ID."},
		},
	}
}

func (a *testSIEMDestinationAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg testSIEMDestinationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	res, err := a.client.TestSIEMDestination(ctx, cfg.DestinationID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("SIEM destination test failed to run", err.Error())
		return
	}
	if !res.Success {
		resp.Diagnostics.AddError("SIEM destination test failed", res.Message)
		return
	}
	resp.SendProgress(action.InvokeProgressEvent{Message: "SIEM destination test succeeded: " + res.Message})
}

// ---- skycloak_test_webhook_subscription ----

var (
	_ action.Action              = (*testWebhookSubscriptionAction)(nil)
	_ action.ActionWithConfigure = (*testWebhookSubscriptionAction)(nil)
)

type testWebhookSubscriptionAction struct{ actionClient }

// NewTestWebhookSubscriptionAction returns the skycloak_test_webhook_subscription action.
func NewTestWebhookSubscriptionAction() action.Action { return &testWebhookSubscriptionAction{} }

type testWebhookSubscriptionModel struct {
	WebhookID types.String `tfsdk:"webhook_id"`
	EventType types.String `tfsdk:"event_type"`
}

func (a *testWebhookSubscriptionAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_webhook_subscription"
}

func (a *testWebhookSubscriptionAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Delivers a sample event to a webhook subscription's endpoint and fails on a non-2xx response.",
		Attributes: map[string]schema.Attribute{
			"webhook_id": schema.StringAttribute{Required: true, MarkdownDescription: "Webhook subscription ID."},
			"event_type": schema.StringAttribute{Required: true, MarkdownDescription: "Event type to send a sample of."},
		},
	}
}

func (a *testWebhookSubscriptionAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg testWebhookSubscriptionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	res, err := a.client.TestWebhookSubscription(ctx, cfg.WebhookID.ValueString(), cfg.EventType.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Webhook test failed to run", err.Error())
		return
	}
	if !res.Success {
		msg := res.ErrorMessage
		if msg == "" {
			msg = fmt.Sprintf("delivery %s returned HTTP %d", res.DeliveryID, res.ResponseCode)
		}
		resp.Diagnostics.AddError("Webhook test delivery failed", msg)
		return
	}
	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Webhook test delivered (%s, HTTP %d, %dms)", res.DeliveryID, res.ResponseCode, res.DurationMs),
	})
}
