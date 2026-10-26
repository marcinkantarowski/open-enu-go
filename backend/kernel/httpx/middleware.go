package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"time"
)

// Middleware is the shape every wrapper here has, so Chain can compose them.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares so that the first listed is the outermost. With
// ROUTE_TRACE set (trace.go) the handler itself is wrapped first, so every
// chain a test builds records what the mux matched.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	h = traced(h)
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// RequestIDHeader is read from the client (a frontend or a proxy may set it)
// and always written back, so a user can quote the id from any failure.
const RequestIDHeader = "X-Request-Id"

type requestIDKey struct{}

var requestIDShape = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestID accepts a well-formed incoming id or mints one, stores it on the
// context, and echoes it in the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !requestIDShape.MatchString(id) {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), id)))
	})
}

// WithRequestID puts a request id on ctx: the middleware's, or - in the
// worker - the one an event or a job carried from the request that queued
// it, so its log lines and audit entries name that request.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom returns the request id, or "" outside a request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is a broken host; a timestamp still gives a usable id.
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Recover turns a panic into a 500 envelope with the stack in the log, so a
// bug in one handler is one failed request, not a dead process.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				slog.ErrorContext(r.Context(), "panic in handler",
					"panic", fmt.Sprint(p), "stack", string(debug.Stack()), "request_id", RequestIDFrom(r.Context()))
				WriteError(w, r, Internal(fmt.Errorf("panic: %v", p)))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// AccessLog writes one JSON line per request with the fields the rest of the
// platform greps for: request_id, method, route (the matched pattern, so
// ids are not in it), status, duration_ms.
func AccessLog(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			route := r.Pattern
			if route == "" {
				route = "(none)"
			}
			log.LogAttrs(r.Context(), levelFor(rec.status), "request",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("route", route),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
		})
	}
}

func levelFor(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Flush keeps SSE working through the recorder.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
