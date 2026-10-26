// Package audit is the record of every write: one Entry per command, success
// or failure, with the before and after state when the handler provided it.
//
// The kernel produces entries (the command bus, the GDPR walker) and stores
// them: Postgres writes audit_entry (kernel migration 0001) once
// platform.WithDatabase is asked to persist, Slog writes them to the log
// before that, and Recorder keeps them for a test. The Audit module reads
// the table and exposes it; it does not decide what is recorded, so coverage
// is real from the first command rather than switched on later - a gap in an
// audit trail is invisible precisely when it matters.
//
// A Logger never fails. Failing to record a change must not undo the change,
// so an implementation swallows its own errors, logs them and carries on.
package audit

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Entry is one recorded change. A value in the kernel rather than a row in a
// module: the kernel records entries and must not depend on whichever module
// stores them.
type Entry struct {
	Action        string         // dotted, stable: `billing.invoice.void`
	SubjectID     string         // the record changed, "" when none
	ActorID       string         // the acting principal, "" for a system action
	OnBehalfOf    string         // the operator behind an impersonated session, "" otherwise
	TenantID      string         // "" when the change was not inside a tenant
	RequestID     string         // the X-Request-Id; in a worker, the one the event or job carried
	Succeeded     bool           // false when the handler returned an error
	FailureReason string         // the error's text when Succeeded is false
	Before        map[string]any // state before, when the handler provided it
	After         map[string]any // state after
	Context       map[string]any // anything else worth keeping: duration_ms, counts
	At            time.Time
}

// Failed returns a copy marked as failed, with the reason.
func (e Entry) Failed(reason string) Entry {
	e.Succeeded = false
	e.FailureReason = reason
	return e
}

// Logger is where entries go. Record must not panic and must not fail the
// caller: the change already happened. Implementations are synchronous -
// Postgres runs its own short transaction before returning - so a slow sink
// slows the command's caller rather than losing the entry.
type Logger interface {
	Record(ctx context.Context, e Entry)
}

// Actor names who is making a change. The kernel has no idea what a user is
// - that is the Identity module's business - so it asks through this. The
// process installs auth.Actor, which reads the verified principal; NoActor
// answers nobody, honestly, where nothing authenticates.
type Actor struct {
	ID         string // the acting principal, "" for a system action
	OnBehalfOf string // the operator behind an impersonation, "" otherwise
}

// ActorProvider resolves the actor from a context.
type ActorProvider interface {
	Actor(ctx context.Context) Actor
}

// NoActor is the provider where nothing authenticates - a test, a one-off
// tool: every change is a system action.
type NoActor struct{}

// Actor returns the empty actor.
func (NoActor) Actor(context.Context) Actor { return Actor{} }

// Slog writes entries to a structured log at Info, under the message "audit".
type Slog struct {
	Log *slog.Logger
}

// Record never panics: a logger that fails is reported to stderr-level slog
// and the entry is dropped, because the alternative is undoing the change.
func (s Slog) Record(ctx context.Context, e Entry) {
	defer func() {
		if r := recover(); r != nil {
			slog.Default().ErrorContext(ctx, "audit.record_failed", "panic", r, "action", e.Action)
		}
	}()
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	attrs := []any{
		"action", e.Action, "subject", e.SubjectID, "actor", e.ActorID, "on_behalf_of", e.OnBehalfOf,
		"tenant", e.TenantID, "request_id", e.RequestID, "succeeded", e.Succeeded,
		"failure", e.FailureReason, "before", e.Before, "after", e.After,
	}
	for k, v := range e.Context {
		attrs = append(attrs, k, v)
	}
	log.InfoContext(ctx, "audit", attrs...)
}

// Recorder keeps entries in memory, for tests that assert on the trail.
type Recorder struct {
	mu      sync.Mutex
	entries []Entry
}

// Record appends.
func (r *Recorder) Record(_ context.Context, e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
}

// Entries returns a copy of what was recorded, in order.
func (r *Recorder) Entries() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Entry(nil), r.entries...)
}

// Reset forgets everything, for the next test.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = nil
}
