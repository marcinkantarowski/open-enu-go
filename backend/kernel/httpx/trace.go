package httpx

import (
	"net/http"
	"os"
	"sync"
)

// RouteTraceEnv names a file every matched route pattern is appended to, one
// per request, as "GET /api/projects/{id}". Set by `make test` and read by
// `openenu route-coverage`, which subtracts the trace from the spec: an
// operation no test reached is a failing build. Coverage is measured at
// runtime because a grep over test sources only proves that somebody wrote a
// name down.
//
// Unset in production and in `make check`; Chain then adds nothing.
const RouteTraceEnv = "ROUTE_TRACE"

var traceMu sync.Mutex

// traced wraps the innermost handler - the mux - because ServeMux writes the
// matched pattern onto the *Request it is handed, and a middleware further out
// may have passed a copy (WithContext does). Innermost also means a request
// the auth middleware refused is not counted: the operation was asked for,
// not exercised.
func traced(h http.Handler) http.Handler {
	path := os.Getenv(RouteTraceEnv)
	if path == "" {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		if r.Pattern == "" {
			return
		}
		traceMu.Lock()
		defer traceMu.Unlock()
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return // a trace that cannot be written fails coverage, loudly, later
		}
		_, _ = f.WriteString(r.Pattern + "\n")
		_ = f.Close()
	})
}
