// Package event is "something happened, stated as fact".
//
// An Event is raised by a handler and dispatched through the outbox (River,
// kernel/outbox): the row that queues it is written in the same transaction
// as the change, so a rollback takes the event with it and a commit
// guarantees delivery. Past tense, always - `invoice.voided`, not
// `void.invoice`; an event is a statement about the world, a command is a
// request, and naming them alike is how the distinction erodes.
//
// An event is internal by default. One that also implements Broadcast is
// pushed to the browser on the tenant's topic by the Broadcaster - opt in,
// never automatic, because the payload then reaches every user of that
// tenant with an open tab.
package event

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/open-enu/kernel/scope"
)

// Event is what a module's contract/ package exports.
type Event interface {
	// EventName is stable and dotted: `billing.invoice.voided`. Webhook
	// subscriptions, browser subscriptions and audit queries key off it, so
	// renaming one breaks consumers outside this repository.
	EventName() string
	// SubjectID is the record the event is about.
	SubjectID() string
	// OccurredAt is when; the raising handler sets it.
	OccurredAt() time.Time
	// Payload is what a subscriber outside this process receives. Explicit
	// rather than reflected: an event crosses a trust boundary when it reaches
	// a browser or a webhook, and "every exported field" is how internals leak.
	Payload() map[string]any
}

// Broadcast marks an event as safe to push to the browser. RequiresPermission
// is an extra permission a viewer needs before the payload is delivered, or
// "" for every member of the tenant.
type Broadcast interface {
	Event
	RequiresPermission() string
}

var nameShape = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// ValidName reports whether a name has the dotted lower-case shape.
func ValidName(name string) bool { return nameShape.MatchString(name) }

// Topic is the Mercure topic a tenant's browsers subscribe to. Per tenant,
// never global: a subscriber's token names this topic and no other.
func Topic(tenantID string) string { return fmt.Sprintf("/tenants/%s/events", tenantID) }

// Envelope is the JSON the browser receives - the keys useAppEvent reads.
type Envelope struct {
	Event              string         `json:"event"`
	SubjectID          string         `json:"subjectId"`
	OccurredAt         string         `json:"occurredAt"`
	Payload            map[string]any `json:"payload"`
	RequiresPermission *string        `json:"requiresPermission"`
}

// NewEnvelope builds the wire form of a broadcast event.
func NewEnvelope(ev Broadcast) Envelope {
	var perm *string
	if p := ev.RequiresPermission(); p != "" {
		perm = &p
	}
	payload := ev.Payload()
	if payload == nil {
		payload = map[string]any{}
	}
	return Envelope{
		Event: ev.EventName(), SubjectID: ev.SubjectID(),
		OccurredAt: ev.OccurredAt().Format(time.RFC3339), Payload: payload, RequiresPermission: perm,
	}
}

// Publisher is the hub: Mercure in the stack, the Recorder in tests. Private
// is always true - a public update would be readable by anyone who guessed
// the tenant id.
type Publisher interface {
	Publish(ctx context.Context, topic string, data []byte) error
}

// Broadcaster pushes Broadcast events to the tenant's topic. A publish
// failure is logged, never returned: the change already happened, and the
// hub being down must not retry a committed write.
type Broadcaster struct {
	Hub Publisher
	Log *slog.Logger
}

// Handle publishes ev when it is a Broadcast and the context has a tenant.
// With no tenant there is no topic anyone is entitled to receive, so the
// event is dropped with a warning rather than invented a global topic for.
func (b Broadcaster) Handle(ctx context.Context, ev Event) {
	bc, ok := ev.(Broadcast)
	if !ok {
		return
	}
	log := b.Log
	if log == nil {
		log = slog.Default()
	}
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		log.WarnContext(ctx, "broadcast.skipped_no_tenant", "event", ev.EventName())
		return
	}
	data, err := json.Marshal(NewEnvelope(bc))
	if err != nil {
		log.ErrorContext(ctx, "broadcast.failed", "event", ev.EventName(), "error", err.Error())
		return
	}
	if err := b.Hub.Publish(ctx, Topic(tenant), data); err != nil {
		log.ErrorContext(ctx, "broadcast.failed", "event", ev.EventName(), "error", err.Error())
	}
}

// LogPublisher is the hub before Mercure is configured: every update is
// logged at Debug and nothing reaches a browser. Dev without the stack.
type LogPublisher struct {
	Log *slog.Logger
}

// Publish logs.
func (p LogPublisher) Publish(ctx context.Context, topic string, data []byte) error {
	log := p.Log
	if log == nil {
		log = slog.Default()
	}
	log.DebugContext(ctx, "broadcast.logged", "topic", topic, "bytes", len(data))
	return nil
}

// Update is one published message, as the Recorder keeps it.
type Update struct {
	Topic string
	Data  []byte
}

// Recorder is a Publisher that keeps every update, for tests.
type Recorder struct {
	mu      sync.Mutex
	updates []Update
	// Err, when set, is returned from Publish - to test the "never throws" path.
	Err error
}

// Publish records.
func (r *Recorder) Publish(_ context.Context, topic string, data []byte) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, Update{Topic: topic, Data: append([]byte(nil), data...)})
	return nil
}

// Updates returns a copy of what was published, in order.
func (r *Recorder) Updates() []Update {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Update(nil), r.updates...)
}
