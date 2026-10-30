package httpx

import (
	"context"
	"io"
	"net/http"
)

// DefaultMaxBody is the request body the API accepts when a route says
// nothing else: far above any JSON form, far below what fills a server's
// memory when sent a thousand times at once.
const DefaultMaxBody int64 = 1 << 20

// MaxBody limits every request body to n bytes. A body over it is a 413 in
// the envelope (code payload_too_large, details.maxBytes) whatever the
// handler was doing with it: the handler's read fails with an
// *http.MaxBytesError and whatever it then tries to answer - a 400 for
// broken JSON, a 500 - is replaced by the 413. A Content-Length already over
// the limit fails the first read, before a byte is buffered.
//
// A route that takes more - an upload - raises its own limit with AllowBody,
// declared where the route is, so the pipeline never lists module paths.
func MaxBody(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}
			b := &limitedBody{rc: r.Body, limit: n, declared: r.ContentLength}
			lw := &bodyLimitWriter{ResponseWriter: w, body: b, r: r}
			r = r.WithContext(context.WithValue(r.Context(), bodyLimitKey{}, b))
			r.Body = b
			lw.r = r
			next.ServeHTTP(lw, r)
		})
	}
}

// AllowBody raises the body limit MaxBody set for the requests it wraps to
// n bytes - on a module's upload route, as a route middleware. It only
// raises, and only before the handler has read anything; without MaxBody in
// front it does nothing.
func AllowBody(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if b, ok := r.Context().Value(bodyLimitKey{}).(*limitedBody); ok && b.read == 0 && n > b.limit {
				b.limit = n
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PayloadTooLarge is the refusal MaxBody answers with.
func PayloadTooLarge(maxBytes int64) *Error {
	return NewError(http.StatusRequestEntityTooLarge, CodePayloadTooLarge, KeyPayloadTooLarge).
		WithDetails(map[string]any{"maxBytes": maxBytes})
}

type bodyLimitKey struct{}

type limitedBody struct {
	rc       io.ReadCloser
	limit    int64
	declared int64 // Content-Length, -1 when unknown
	read     int64
	exceeded bool
}

func (b *limitedBody) Read(p []byte) (int, error) {
	if b.exceeded || b.declared > b.limit {
		b.exceeded = true
		return 0, &http.MaxBytesError{Limit: b.limit}
	}
	// One byte past the limit is read to tell "exactly the limit" from "over".
	if room := b.limit - b.read + 1; int64(len(p)) > room {
		p = p[:room]
	}
	n, err := b.rc.Read(p)
	b.read += int64(n)
	if b.read > b.limit {
		b.exceeded = true
		return n - int(b.read-b.limit), &http.MaxBytesError{Limit: b.limit}
	}
	return n, err
}

func (b *limitedBody) Close() error { return b.rc.Close() }

// bodyLimitWriter replaces the handler's answer with the 413 once the body
// went over the limit.
type bodyLimitWriter struct {
	http.ResponseWriter
	body       *limitedBody
	r          *http.Request
	wrote      bool
	overridden bool
}

func (w *bodyLimitWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.wrote = true
	if w.body.exceeded {
		w.overridden = true
		w.Header().Del("Content-Length")
		WriteError(w.ResponseWriter, w.r, PayloadTooLarge(w.body.limit))
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *bodyLimitWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.overridden {
		return len(p), nil
	}
	return w.ResponseWriter.Write(p)
}

// Flush keeps a streamed answer streaming through the wrapper.
func (w *bodyLimitWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok && !w.overridden {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the connection.
func (w *bodyLimitWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
