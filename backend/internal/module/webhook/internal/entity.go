package internal

import (
	"time"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/i18n"
)

const (
	statusPending   = "pending"
	statusDelivered = "delivered"
	statusFailed    = "failed"

	// maxAttempts is the queue's (outbox.Enqueue: five) and the log's: a
	// delivery is `pending` until its attempts are spent, then `failed`.
	// Showing a queued retry as a failure sends people chasing a non-problem.
	maxAttempts = 5

	// NotificationDeliveryFailed is raised to the tenant on the fifth failure.
	NotificationDeliveryFailed = "webhook.delivery_failed"

	keyHTTPSRequired  i18n.Key = "webhook.error.https_required"
	keyEventsRequired i18n.Key = "webhook.error.events_required"
	keyNoTenant       i18n.Key = "webhook.error.no_tenant"
	keySecretWarning  i18n.Key = "webhook.secret.warning"
)

// endpoint is a URL, the event names it wants, and a signing secret shown
// exactly once. Deactivated, never deleted: the log points at it.
type endpoint struct {
	ID        string
	TenantID  string
	URL       string
	Events    []string
	Secret    string // plaintext here; ciphertext in the table
	Active    bool
	Version   int
	CreatedAt time.Time
}

func (e endpoint) wants(eventName string) bool {
	if !e.Active {
		return false
	}
	for _, n := range e.Events {
		if n == eventName {
			return true
		}
	}
	return false
}

// Snapshot never holds the secret: the trail is read by more people than
// the record, and the secret exists in clear exactly once.
func (e endpoint) Snapshot() map[string]any {
	return map[string]any{"id": e.ID, "url": e.URL, "events": e.Events, "active": e.Active, "version": e.Version}
}

func (e endpoint) view() WebhookEndpoint {
	return WebhookEndpoint{Id: e.ID, Url: e.URL, Events: e.Events, Active: e.Active, Version: e.Version, CreatedAt: e.CreatedAt}
}

// created is the create command's result: the endpoint and, this once, its
// secret in clear.
type created struct {
	endpoint endpoint
	secret   string
}

// delivery is one event's attempt history against one endpoint. The log is
// the product: "did you send it?" is the only question anyone asks.
type delivery struct {
	ID            string
	TenantID      string
	EndpointID    string
	EventName     string
	Payload       map[string]any
	Status        string
	Attempts      int
	ResponseCode  *int
	ResponseBody  *string
	LastAttemptAt *time.Time
	CreatedAt     time.Time
}

func newDelivery(tenantID, endpointID, eventName string, payload map[string]any) delivery {
	return delivery{ID: uuid.Must(uuid.NewV7()).String(), TenantID: tenantID, EndpointID: endpointID, EventName: eventName, Payload: payload,
		Status: statusPending, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

func (d delivery) Snapshot() map[string]any {
	return map[string]any{"id": d.ID, "endpointId": d.EndpointID, "event": d.EventName, "status": d.Status, "attempts": d.Attempts}
}

func (d delivery) view() WebhookDelivery {
	return WebhookDelivery{Id: d.ID, EndpointId: d.EndpointID, Event: d.EventName, Status: WebhookDeliveryStatus(d.Status), Attempts: d.Attempts,
		ResponseCode: d.ResponseCode, ResponseBody: d.ResponseBody, LastAttemptAt: d.LastAttemptAt, CreatedAt: d.CreatedAt}
}
