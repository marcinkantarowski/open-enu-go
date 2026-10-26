package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/open-enu/kernel/notification"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/scope"
)

// deliverJob is the background half: an id, never the row - it runs in
// another process at another time, and the row may have changed. The tenant
// travels with the outbox.
type deliverJob struct {
	DeliveryID string `json:"deliveryId"`
}

// Kind is River's job kind, dotted by module.
func (deliverJob) Kind() string { return "webhook.deliver" }

// requestTimeout bounds one attempt; a receiver that takes longer is a
// failed attempt, not a stuck worker.
const requestTimeout = 10 * time.Second

// deliverer runs one attempt. Its writes are the log's own, outside the bus
// (repository.succeeded / failed); the error it returns is what makes River
// retry, so the queue and the log count the same attempts.
type deliverer struct {
	repo     repository
	http     *http.Client
	notifier notification.Notifier
	log      *slog.Logger
}

// publisher is the outbox subscriber for EVERY event: which events a tenant
// wants is a row in its table, not a line here. It must not fail - it runs
// beside the broadcaster, and a webhook nobody is listening for must not
// fail the event that carried it.
func (d deliverer) publish(ctx context.Context, ev outbox.Delivered) error {
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		// No tenant means no endpoints to consult; the event itself was valid.
		return nil
	}
	endpoints, err := d.repo.subscribedTo(ctx, ev.EventName())
	if err != nil {
		d.log.ErrorContext(ctx, "webhook.publish_failed", "event", ev.EventName(), "error", err.Error())
		return nil
	}
	for _, e := range endpoints {
		// `Payload()`, not the event: what crosses to a third party is a
		// decision the event makes explicitly.
		payload := map[string]any{"event": ev.EventName(), "subjectId": ev.SubjectID(), "occurredAt": ev.OccurredAt().UTC().Format(time.RFC3339), "data": ev.Payload()}
		if err := d.repo.record(ctx, newDelivery(tenant, e.ID, ev.EventName(), payload)); err != nil {
			d.log.ErrorContext(ctx, "webhook.publish_failed", "event", ev.EventName(), "endpoint", e.ID, "error", err.Error())
		}
	}
	return nil
}

// run is one attempt against one delivery.
func (d deliverer) run(ctx context.Context, j deliverJob) error {
	del, err := d.repo.findDelivery(ctx, j.DeliveryID)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil // gone while it sat in the queue; nothing to retry
		}
		return err
	}
	e, err := d.repo.findEndpoint(ctx, del.EndpointID)
	if err != nil || !e.Active {
		// Deactivated or deleted while this sat in the queue. Nothing to do
		// and nothing to retry - an error would retry it five times first.
		if err != nil && !errors.Is(err, errNotFound) {
			return err
		}
		return nil
	}
	body, err := json.Marshal(del.Payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.URL, bytes.NewReader(body))
	if err != nil {
		return d.fail(ctx, del, e, nil, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers(del.ID, time.Now().Unix(), body, e.Secret) {
		req.Header.Set(k, v)
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return d.fail(ctx, del, e, nil, err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return d.repo.succeeded(ctx, del.ID, resp.StatusCode)
	}
	// Anything else is a failure the receiver reported; its body is what the
	// tenant will ask about.
	reason, _ := io.ReadAll(io.LimitReader(resp.Body, responseBodyLimit))
	return d.fail(ctx, del, e, &resp.StatusCode, string(reason))
}

// fail records the attempt, tells the tenant when the retries are spent, and
// returns the error that makes River try again.
func (d deliverer) fail(ctx context.Context, del delivery, e endpoint, code *int, reason string) error {
	after, err := d.repo.failed(ctx, del.ID, code, reason)
	if err != nil {
		return err
	}
	if after.Status == statusFailed {
		// Out of retries. Nobody finds out otherwise - a dead endpoint is
		// silent by definition, and the tenant notices days later when data
		// is missing. Still retrying would be noise people mute.
		if nerr := d.notifier.ToTenant(ctx, del.TenantID, NotificationDeliveryFailed, map[string]any{"event": del.EventName, "url": e.URL, "attempts": after.Attempts}); nerr != nil {
			d.log.ErrorContext(ctx, "webhook.notify_failed", "delivery", del.ID, "error", nerr.Error())
		}
	}
	if code != nil {
		return fmt.Errorf("webhook: delivery %s failed with %d: %s", del.ID, *code, reason)
	}
	return fmt.Errorf("webhook: delivery %s failed: %s", del.ID, reason)
}
