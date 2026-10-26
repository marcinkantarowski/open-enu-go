// Package httpx is what every HTTP response has in common: the one error
// envelope, the 409 body, the request id, the health endpoints, the access
// log, panic recovery, and a server with timeouts and graceful shutdown.
//
// The wire shapes are a contract with the browser apps: `useApi` in
// @open-enu/ui-kit parses `error.code`, `error.details` and
// `error.requestId`, and a change to any of them breaks every app at once.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/open-enu/kernel/i18n"
)

// Error is the one failure shape. `Code` is the stable machine token clients
// branch on; `Message` is a translation key, resolved at write time, and may
// change - nothing should branch on it.
type Error struct {
	Status  int
	Code    string
	Message i18n.Key
	Details map[string]any
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%d %s: %v", e.Status, e.Code, e.cause)
	}
	return fmt.Sprintf("%d %s", e.Status, e.Code)
}

func (e *Error) Unwrap() error { return e.cause }

// NewError builds an Error. Message is an i18n.Key, not a sentence: the
// rawstring analyzer refuses a string literal here.
func NewError(status int, code string, message i18n.Key) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// WithDetails attaches structured data the client can act on (a validation
// map, the two versions of a conflict). Never prose.
func (e *Error) WithDetails(details map[string]any) *Error {
	c := *e
	c.Details = details
	return &c
}

// WithCause keeps the underlying error for the log; it is never sent.
func (e *Error) WithCause(err error) *Error {
	c := *e
	c.cause = err
	return &c
}

// The codes each status maps to. Stable strings: a frontend branches on
// them, so renaming one is a breaking change to the API.
const (
	CodeBadRequest         = "bad_request"
	CodeUnauthenticated    = "unauthenticated"
	CodeForbidden          = "forbidden"
	CodeNotFound           = "not_found"
	CodeMethodNotAllowed   = "method_not_allowed"
	CodeConflict           = "conflict"
	CodeValidationFailed   = "validation_failed"
	CodeRateLimited        = "rate_limited"
	CodeServiceUnavailable = "service_unavailable"
	CodeInternal           = "internal_error"
)

// Kernel-owned messages. Modules declare their own keys beside their code.
const (
	KeyNotFound           i18n.Key = "kernel.error.not_found"
	KeyForbidden          i18n.Key = "kernel.error.forbidden"
	KeyUnauthenticated    i18n.Key = "kernel.error.unauthenticated"
	KeyBadRequest         i18n.Key = "kernel.error.bad_request"
	KeyConflict           i18n.Key = "kernel.error.conflict"
	KeyValidationFailed   i18n.Key = "kernel.error.validation_failed"
	KeyRateLimited        i18n.Key = "kernel.error.rate_limited"
	KeyServiceUnavailable i18n.Key = "kernel.error.service_unavailable"
	KeyInternal           i18n.Key = "kernel.error.internal"
)

func NotFound(message i18n.Key) *Error { return NewError(http.StatusNotFound, CodeNotFound, message) }
func Forbidden(message i18n.Key) *Error {
	return NewError(http.StatusForbidden, CodeForbidden, message)
}
func Unauthenticated(message i18n.Key) *Error {
	return NewError(http.StatusUnauthorized, CodeUnauthenticated, message)
}
func BadRequest(message i18n.Key) *Error {
	return NewError(http.StatusBadRequest, CodeBadRequest, message)
}

// ValidationFailed carries field -> message-key violations under
// details.violations.
func ValidationFailed(violations map[string]i18n.Key) *Error {
	v := make(map[string]any, len(violations))
	for field, key := range violations {
		v[field] = string(key)
	}
	return NewError(http.StatusUnprocessableEntity, CodeValidationFailed, KeyValidationFailed).
		WithDetails(map[string]any{"violations": v})
}

// Internal wraps an unexpected error. The cause goes to the log with the
// request id; the client sees only the code and the id to quote.
func Internal(cause error) *Error {
	return NewError(http.StatusInternalServerError, CodeInternal, KeyInternal).WithCause(cause)
}

// ConflictBody is what a 409 carries in details: enough for the conflict bar
// to offer reload / overwrite / diff instead of "your work is lost".
type ConflictBody struct {
	Resource       string `json:"resource"`
	ResourceID     string `json:"resourceId,omitempty"`
	YourVersion    any    `json:"yourVersion"`
	CurrentVersion any    `json:"currentVersion"`
	Current        any    `json:"current,omitempty"`
}

// Conflict is the optimistic-locking refusal.
func Conflict(body ConflictBody) *Error {
	d := map[string]any{
		"resource":       body.Resource,
		"yourVersion":    body.YourVersion,
		"currentVersion": body.CurrentVersion,
	}
	if body.ResourceID != "" {
		d["resourceId"] = body.ResourceID
	}
	if body.Current != nil {
		d["current"] = body.Current
	}
	return NewError(http.StatusConflict, CodeConflict, KeyConflict).WithDetails(d)
}

// Translator resolves a key for the request's locale. The Locale middleware
// (locale.go) installs one on the context; without it the key itself is sent, which
// is honest and greppable rather than a guessed English sentence.
type Translator func(ctx context.Context, key i18n.Key) string

type translatorKey struct{}

// WithTranslator returns a context carrying t.
func WithTranslator(ctx context.Context, t Translator) context.Context {
	return context.WithValue(ctx, translatorKey{}, t)
}

func translate(ctx context.Context, key i18n.Key) string {
	if t, ok := ctx.Value(translatorKey{}).(Translator); ok && t != nil {
		return t(ctx, key)
	}
	return string(key)
}

type envelope struct {
	Error envelopeBody `json:"error"`
}

type envelopeBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"requestId,omitempty"`
}

// WriteError renders any error as the envelope. An *Error is sent as
// declared; anything else is a 500 whose cause is logged, never sent.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = Internal(err)
	}
	if e.Status >= 500 {
		slog.ErrorContext(r.Context(), "request failed",
			"status", e.Status, "code", e.Code, "error", errorString(e), "request_id", RequestIDFrom(r.Context()))
	}
	WriteJSON(w, e.Status, envelope{Error: envelopeBody{
		Code:      e.Code,
		Message:   translate(r.Context(), e.Message),
		Details:   e.Details,
		RequestID: RequestIDFrom(r.Context()),
	}})
}

func errorString(e *Error) string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return e.Code
}

// WriteJSON encodes v with the content type set. Encoding failures are a
// programming error and surface as a 500 envelope.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		body, _ = json.Marshal(envelope{Error: envelopeBody{Code: CodeInternal, Message: string(KeyInternal)}})
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// NotFoundHandler is the catch-all mounted at "/" so that an unknown path
// gets the envelope rather than net/http's text/plain.
func NotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, NotFound(KeyNotFound))
	})
}
