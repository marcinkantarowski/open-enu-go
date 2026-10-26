// Package notification is the contract for telling somebody that something
// happened. Two audiences, because they are different questions: ToUser is
// "this concerns you personally", ToTenant is "this concerns the workspace"
// and fans out to whoever should see it - a module raising a notification
// should not have to decide who that is.
//
// The context is translation arguments, never prose: the message is a pair
// of translation keys on the declaration, so one row reads correctly in each
// recipient's own language (ADR-0008). A type is declared on the
// module (module.Module.Notifications) for the same reason a flag is: an
// undeclared type has no key anyone can translate and nothing a person
// could switch off.
//
// The kernel ships Log, so code written against the contract works before
// the Notification module exists; that module provides the real Notifier
// through module.Module.Notifier and no caller changes.
package notification

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/open-enu/kernel/i18n"
)

// Definition is one kind of notification a module can raise.
type Definition struct {
	// Type is dotted and prefixed with the owning module: `webhook.delivery_failed`.
	Type string
	// TitleKey and BodyKey are translation keys the module's own catalogue
	// holds; the client translates them with the row's context as arguments.
	TitleKey i18n.Key
	BodyKey  i18n.Key
	// Email sends a mail as well as writing the feed row. Worth it only when
	// the point is to reach somebody who is not looking at the app.
	Email bool
}

// Validate is the shape check the registry runs.
func (d Definition) Validate() error {
	if !i18n.Key(d.Type).Valid() {
		return fmt.Errorf("notification type %q must be dotted lowercase", d.Type)
	}
	if !d.TitleKey.Valid() || !d.BodyKey.Valid() {
		return fmt.Errorf("notification %q: titleKey and bodyKey must be catalogue keys, not sentences", d.Type)
	}
	return nil
}

// Notifier tells someone. Context values are scalars - they become
// translation arguments.
type Notifier interface {
	ToUser(ctx context.Context, userID, typ string, context map[string]any) error
	ToTenant(ctx context.Context, tenantID, typ string, context map[string]any) error
}

// Log writes notifications to the log. Usable rather than a no-op, like the
// log mailer: output is visible in `make logs` before any module stores it.
type Log struct{ Log *slog.Logger }

func (l Log) ToUser(ctx context.Context, userID, typ string, c map[string]any) error {
	l.Log.InfoContext(ctx, "notification", "to", "user:"+userID, "type", typ, "context", c)
	return nil
}

func (l Log) ToTenant(ctx context.Context, tenantID, typ string, c map[string]any) error {
	l.Log.InfoContext(ctx, "notification", "to", "tenant:"+tenantID, "type", typ, "context", c)
	return nil
}

// Sent is one recorded notification.
type Sent struct {
	To      string // "user:<id>" or "tenant:<id>"
	Type    string
	Context map[string]any
}

// Recorder keeps what was raised, for tests.
type Recorder struct {
	mu   sync.Mutex
	sent []Sent
}

func (r *Recorder) ToUser(_ context.Context, userID, typ string, c map[string]any) error {
	return r.add("user:"+userID, typ, c)
}

func (r *Recorder) ToTenant(_ context.Context, tenantID, typ string, c map[string]any) error {
	return r.add("tenant:"+tenantID, typ, c)
}

func (r *Recorder) add(to, typ string, c map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, Sent{To: to, Type: typ, Context: c})
	return nil
}

// Sent returns everything raised so far, oldest first.
func (r *Recorder) Sent() []Sent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Sent(nil), r.sent...)
}
