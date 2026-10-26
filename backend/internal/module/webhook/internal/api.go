package internal

import (
	"context"
	"strings"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/i18n"
)

// deliveryLogLimit is a screen's worth of the log.
const deliveryLogLimit = 50

// api implements the StrictServerInterface genapi derives from
// ../openapi.json. Reads go through the repository; writes are commands.
type api struct {
	bus   *command.Bus
	repo  repository
	guard auth.Guard
	i18n  *i18n.Catalogue
}

var _ StrictServerInterface = api{}

func (a api) GetWebhookIndex(ctx context.Context, _ GetWebhookIndexRequestObject) (GetWebhookIndexResponseObject, error) {
	if _, err := auth.Require(ctx, "webhook.view"); err != nil {
		return nil, err
	}
	items, err := a.repo.endpoints(ctx)
	if err != nil {
		return nil, err
	}
	out := WebhookEndpointList{Items: make([]WebhookEndpoint, 0, len(items))}
	for _, e := range items {
		out.Items = append(out.Items, e.view())
	}
	return GetWebhookIndex200JSONResponse(out), nil
}

func (a api) PostWebhookCreate(ctx context.Context, req PostWebhookCreateRequestObject) (PostWebhookCreateResponseObject, error) {
	if _, err := auth.Require(ctx, "webhook.manage"); err != nil {
		return nil, err
	}
	// Support may look at where a tenant's events go; support may not add a
	// destination for them while wearing that tenant's face.
	if err := a.guard.DenyImpersonated(ctx, "webhook.endpoint.created"); err != nil {
		return nil, err
	}
	var events []string
	for _, e := range req.Body.Events {
		if e = strings.TrimSpace(e); e != "" {
			events = append(events, e)
		}
	}
	c, err := command.Dispatch[created](ctx, a.bus, createEndpoint{URL: strings.TrimSpace(req.Body.Url), Events: events})
	if err != nil {
		return nil, err
	}
	v := c.endpoint.view()
	// The only time the secret leaves the server: encrypted at rest and never
	// returned again, which is the correct answer to "can you resend it?".
	return PostWebhookCreate201JSONResponse{Id: v.Id, Url: v.Url, Events: v.Events, Active: v.Active, Version: v.Version, CreatedAt: v.CreatedAt,
		Secret: c.secret, Warning: a.i18n.TranslateCtx(ctx, keySecretWarning)}, nil
}

func (a api) DeleteWebhookDeactivate(ctx context.Context, req DeleteWebhookDeactivateRequestObject) (DeleteWebhookDeactivateResponseObject, error) {
	if _, err := auth.Require(ctx, "webhook.manage"); err != nil {
		return nil, err
	}
	if err := a.guard.DenyImpersonated(ctx, "webhook.endpoint.deactivated"); err != nil {
		return nil, err
	}
	e, err := command.Dispatch[endpoint](ctx, a.bus, deactivateEndpoint{ID: req.Id})
	if err != nil {
		return nil, err
	}
	return DeleteWebhookDeactivate200JSONResponse(e.view()), nil
}

func (a api) GetWebhookDeliveries(ctx context.Context, req GetWebhookDeliveriesRequestObject) (GetWebhookDeliveriesResponseObject, error) {
	if _, err := auth.Require(ctx, "webhook.view"); err != nil {
		return nil, err
	}
	endpointID := ""
	if req.Params.EndpointId != nil {
		endpointID = *req.Params.EndpointId
	}
	items, err := a.repo.recent(ctx, endpointID, deliveryLogLimit)
	if err != nil {
		return nil, err
	}
	out := WebhookDeliveryList{Items: make([]WebhookDelivery, 0, len(items))}
	for _, d := range items {
		out.Items = append(out.Items, d.view())
	}
	return GetWebhookDeliveries200JSONResponse(out), nil
}

func (a api) PostWebhookRedeliver(ctx context.Context, req PostWebhookRedeliverRequestObject) (PostWebhookRedeliverResponseObject, error) {
	if _, err := auth.Require(ctx, "webhook.manage"); err != nil {
		return nil, err
	}
	// 202: queued, not sent. The log is where the outcome appears.
	d, err := command.Dispatch[delivery](ctx, a.bus, redeliver{DeliveryID: req.Id})
	if err != nil {
		return nil, err
	}
	return PostWebhookRedeliver202JSONResponse(d.view()), nil
}
