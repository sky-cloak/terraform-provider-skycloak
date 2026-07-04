package skycloak

import (
	"context"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"

	"github.com/sky-cloak/terraform-provider-skycloak/internal/apiclient"
)

// WebhookSubscription is a saved webhook subscription for the workspace.
// AuthorizationHeader and SigningSecret are write-only; reads report the
// Has* booleans instead.
type WebhookSubscription struct {
	ID                     string
	Name                   string
	URL                    string
	Enabled                bool
	Source                 string
	EventTypes             []string
	ClusterID              string
	RealmID                string
	HasAuthorizationHeader bool
	HasSigningSecret       bool
	CreatedAt              string
	UpdatedAt              string
}

// WebhookSubscriptionRequest holds the desired subscription state.
type WebhookSubscriptionRequest struct {
	Name                string
	URL                 string
	Enabled             *bool
	Source              string
	EventTypes          []string
	ClusterID           string
	RealmID             string
	SigningSecret       string
	AuthorizationHeader string
}

// WebhookEventType describes one event type webhooks can subscribe to.
type WebhookEventType struct {
	Type        string
	Category    string
	Description string
	Deprecated  bool
}

// WebhookTestResult is the outcome of a test delivery.
type WebhookTestResult struct {
	DeliveryID   string
	Success      bool
	ResponseCode int64
	ResponseBody string
	ErrorMessage string
	DurationMs   int64
	AttemptedAt  string
}

func wid(s string) apiclient.WebhookId {
	id, _ := uuid.Parse(s)
	return id
}

func webhookFromAPI(w *apiclient.WebhookSubscription) *WebhookSubscription {
	out := &WebhookSubscription{
		ID: w.Id.String(), Name: string(w.Name), URL: w.Url, Enabled: w.Enabled,
		Source: string(w.Source), EventTypes: w.EventTypes,
		HasAuthorizationHeader: w.HasAuthorizationHeader, HasSigningSecret: w.HasSigningSecret,
		RealmID:   strDerefPtr(w.RealmId),
		CreatedAt: fmtTime(w.CreatedAt), UpdatedAt: fmtTime(w.UpdatedAt),
	}
	if w.ClusterId != nil {
		out.ClusterID = w.ClusterId.String()
	}
	return out
}

// ListWebhookSubscriptions returns the workspace's webhook subscriptions.
func (c *Client) ListWebhookSubscriptions(ctx context.Context) ([]WebhookSubscription, error) {
	// Webhook routes carry no version header parameter; the request editor
	// still sets API-Version. Params here are optional server-side filters.
	resp, err := c.gen.ListWebhookSubscriptionsWithResponse(ctx, &apiclient.ListWebhookSubscriptionsParams{})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]WebhookSubscription, 0, len(*resp.JSON200))
	for i := range *resp.JSON200 {
		out = append(out, *webhookFromAPI(&(*resp.JSON200)[i]))
	}
	return out, nil
}

// GetWebhookSubscription returns a single subscription by ID.
func (c *Client) GetWebhookSubscription(ctx context.Context, id string) (*WebhookSubscription, error) {
	resp, err := c.gen.GetWebhookSubscriptionWithResponse(ctx, wid(id))
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return webhookFromAPI(resp.JSON200), nil
}

// CreateWebhookSubscription creates a subscription. SigningSecret is required.
func (c *Client) CreateWebhookSubscription(ctx context.Context, req WebhookSubscriptionRequest) (*WebhookSubscription, error) {
	body := apiclient.CreateWebhookSubscriptionJSONRequestBody{
		Name: req.Name, Url: req.URL, Enabled: req.Enabled,
		Source: apiclient.WebhookSource(req.Source), EventTypes: req.EventTypes,
		SigningSecret: req.SigningSecret,
	}
	if req.ClusterID != "" {
		id := cid(req.ClusterID)
		body.ClusterId = &id
	}
	if req.RealmID != "" {
		id, _ := uuid.Parse(req.RealmID)
		rid := apiclient.RealmId(id)
		body.RealmId = &rid
	}
	if req.AuthorizationHeader != "" {
		h := req.AuthorizationHeader
		body.AuthorizationHeader = &h
	}
	resp, err := c.gen.CreateWebhookSubscriptionWithResponse(ctx, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return webhookFromAPI(resp.JSON201), nil
}

// UpdateWebhookSubscription converges a subscription on the desired state.
// Empty SigningSecret/AuthorizationHeader retain the stored values; an empty
// ClusterID/RealmID clears the scoping.
func (c *Client) UpdateWebhookSubscription(ctx context.Context, id string, req WebhookSubscriptionRequest) (*WebhookSubscription, error) {
	name := apiclient.WebhookSubscriptionName(req.Name)
	url := apiclient.WebhookUrl(req.URL)
	src := apiclient.WebhookSource(req.Source)
	evts := req.EventTypes
	body := apiclient.UpdateWebhookSubscriptionJSONRequestBody{
		Name: &name, Url: &url, Enabled: req.Enabled, Source: &src, EventTypes: &evts,
	}
	if req.ClusterID != "" {
		body.ClusterId = nullable.NewNullableWithValue(cid(req.ClusterID))
	} else {
		body.ClusterId = nullable.NewNullNullable[apiclient.ClusterId]()
	}
	if req.RealmID != "" {
		rid, _ := uuid.Parse(req.RealmID)
		body.RealmId = nullable.NewNullableWithValue(apiclient.RealmId(rid))
	} else {
		body.RealmId = nullable.NewNullNullable[apiclient.RealmId]()
	}
	if req.SigningSecret != "" {
		s := req.SigningSecret
		body.SigningSecret = &s
	}
	if req.AuthorizationHeader != "" {
		body.AuthorizationHeader = nullable.NewNullableWithValue(apiclient.WebhookAuthorizationHeader(req.AuthorizationHeader))
	}
	resp, err := c.gen.UpdateWebhookSubscriptionWithResponse(ctx, wid(id), body)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	return webhookFromAPI(resp.JSON200), nil
}

// DeleteWebhookSubscription deletes a subscription.
func (c *Client) DeleteWebhookSubscription(ctx context.Context, id string) error {
	resp, err := c.gen.DeleteWebhookSubscriptionWithResponse(ctx, wid(id))
	if err != nil {
		return err
	}
	if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
		return statusError(resp.HTTPResponse, resp.Body)
	}
	return nil
}

// TestWebhookSubscription sends a sample event of the given type.
func (c *Client) TestWebhookSubscription(ctx context.Context, id, eventType string) (*WebhookTestResult, error) {
	resp, err := c.gen.TestWebhookSubscriptionWithResponse(ctx, wid(id), apiclient.TestWebhookSubscriptionRequest{EventType: eventType})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	r := resp.JSON200
	out := &WebhookTestResult{
		DeliveryID: r.DeliveryId, Success: r.Success, DurationMs: r.DurationMs,
		ResponseBody: strDerefPtr(r.ResponseBody), ErrorMessage: strDerefPtr(r.ErrorMessage),
		AttemptedAt: fmtTime(r.AttemptedAt),
	}
	if r.ResponseCode != nil {
		out.ResponseCode = int64(*r.ResponseCode)
	}
	return out, nil
}

// ListWebhookEventTypes returns the catalog of subscribable event types.
func (c *Client) ListWebhookEventTypes(ctx context.Context) ([]WebhookEventType, error) {
	resp, err := c.gen.ListWebhookEventTypesWithResponse(ctx, &apiclient.ListWebhookEventTypesParams{})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, statusError(resp.HTTPResponse, resp.Body)
	}
	out := make([]WebhookEventType, 0, len(*resp.JSON200))
	for _, e := range *resp.JSON200 {
		out = append(out, WebhookEventType{
			Type: e.Type, Category: string(e.Category), Description: e.Description, Deprecated: e.Deprecated,
		})
	}
	return out, nil
}
