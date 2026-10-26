// Package logctx puts what a context knows on every log line logged with it:
// which request, which tenant, who, which command. A line without them is a
// line nobody can join to anything - not to the failure a user reports with
// the X-Request-Id they were shown, not to the job that ran twenty minutes
// later because of it.
//
// It is one slog.Handler wrapped around the process's own. The kernel's
// fields are Standard(); a module adds its own through Module.LogFields.
// Only lines logged WITH a context get them - slog.InfoContext(ctx, ...),
// log.LogAttrs(ctx, ...) - which is why the kernel logs that way.
package logctx

import (
	"context"
	"log/slog"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/command"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/scope"
)

// Field returns the attributes ctx contributes to a line, or none. It runs on
// every line, so it reads the context and nothing else.
type Field func(ctx context.Context) []slog.Attr

// Handler wraps next so that every record logged with a context carries each
// field's attributes. A key the line already has - set by the caller, or by
// Logger.With - is not added a second time. Inside a WithGroup, the fields
// land in that group, as any attribute added there would.
func Handler(next slog.Handler, fields ...Field) slog.Handler {
	return &handler{next: next, fields: fields}
}

type handler struct {
	next   slog.Handler
	fields []Field
	preset []string // keys already set through WithAttrs
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	if ctx == nil || len(h.fields) == 0 {
		return h.next.Handle(ctx, r)
	}
	var extra []slog.Attr
	has := map[string]bool{}
	for _, k := range h.preset {
		has[k] = true
	}
	r.Attrs(func(a slog.Attr) bool { has[a.Key] = true; return true })
	for _, f := range h.fields {
		for _, a := range f(ctx) {
			if !has[a.Key] {
				has[a.Key] = true
				extra = append(extra, a)
			}
		}
	}
	if len(extra) > 0 {
		// Records share state between copies; a modified one is cloned.
		r = r.Clone()
		r.AddAttrs(extra...)
	}
	return h.next.Handle(ctx, r)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	preset := append([]string(nil), h.preset...)
	for _, a := range attrs {
		preset = append(preset, a.Key)
	}
	return &handler{next: h.next.WithAttrs(attrs), fields: h.fields, preset: preset}
}

func (h *handler) WithGroup(name string) slog.Handler {
	// Keys set before the group are a different attribute from one of the
	// same name inside it.
	return &handler{next: h.next.WithGroup(name), fields: h.fields}
}

// Standard is what the kernel knows on every line: the request, the tenant
// in scope, the caller and the command running.
func Standard() []Field { return []Field{Request, Tenant, Caller, Command} }

// Request is the X-Request-Id - in the api process from the request, in the
// worker from the event or job, which carry the id of the request that
// queued them.
func Request(ctx context.Context) []slog.Attr {
	if id := httpx.RequestIDFrom(ctx); id != "" {
		return []slog.Attr{slog.String("request_id", id)}
	}
	return nil
}

// Tenant is the tenant in scope, when there is one.
func Tenant(ctx context.Context) []slog.Attr {
	if t, ok := scope.TenantID(ctx); ok && t != "" {
		return []slog.Attr{slog.String("tenant_id", t)}
	}
	return nil
}

// Caller says who is calling, by kind: a person (user_id), a machine
// (api_key_id), an operator (operator_id), and the operator behind an
// impersonated session (impersonator_id).
func Caller(ctx context.Context) []slog.Attr {
	p, ok := auth.From(ctx)
	if !ok || p.Subject == "" {
		return nil
	}
	key := "user_id"
	switch {
	case p.APIKey:
		key = "api_key_id"
	case p.Realm == auth.RealmManager:
		key = "operator_id"
	}
	out := []slog.Attr{slog.String(key, p.Subject)}
	if p.Impersonator != "" {
		out = append(out, slog.String("impersonator_id", p.Impersonator))
	}
	return out
}

// Command is the audit action of the command whose handler is running.
func Command(ctx context.Context) []slog.Attr {
	if c := command.Running(ctx); c != "" {
		return []slog.Attr{slog.String("command", c)}
	}
	return nil
}
