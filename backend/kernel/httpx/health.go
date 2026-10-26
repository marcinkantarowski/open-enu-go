package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"time"
)

// Check answers "is this dependency usable right now?". The api process
// registers one per backing service it was given (cmd/openenu/api.go), so
// that /health/deep reports what it verified and never implies more.
type Check func(ctx context.Context) error

// Health serves the two probes:
//
//	/health       shallow - "is this process serving?". Polled by Traefik and
//	              the container healthcheck every few seconds, so it touches
//	              nothing.
//	/health/deep  readiness - runs every registered Check with a short timeout
//	              and answers 503 naming what failed.
type Health struct {
	Stage   string
	Kernel  string
	Checks  map[string]Check
	Timeout time.Duration // per check; 2s when zero
}

// Mount registers both probes outside the API prefix - they are not part of
// the contract and never were.
func (h *Health) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.shallow)
	mux.HandleFunc("GET /health/deep", h.deep)
}

func (h *Health) shallow(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"stage":  h.Stage,
		"kernel": h.Kernel,
	})
}

type checkResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Ms    int64  `json:"ms"`
}

func (h *Health) deep(w http.ResponseWriter, r *http.Request) {
	timeout := h.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	results := map[string]checkResult{}
	failed := []string{}
	for name, check := range h.Checks {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		start := time.Now()
		err := check(ctx)
		cancel()
		res := checkResult{OK: err == nil, Ms: time.Since(start).Milliseconds()}
		if err != nil {
			// The endpoint is public. The driver's message names hosts,
			// ports and sometimes the DSN; that goes to the log, and the
			// body says only which check failed and whether it timed out.
			res.Error = "unavailable"
			if errors.Is(err, context.DeadlineExceeded) {
				res.Error = "timeout"
			}
			slog.WarnContext(r.Context(), "health check failed", "check", name, "error", err.Error(), "request_id", RequestIDFrom(r.Context()))
			failed = append(failed, name)
		}
		results[name] = res
	}
	sort.Strings(failed)
	status, state := http.StatusOK, "ok"
	if len(failed) > 0 {
		status, state = http.StatusServiceUnavailable, "degraded"
	}
	WriteJSON(w, status, map[string]any{
		"status": state,
		"stage":  h.Stage,
		"kernel": h.Kernel,
		"checks": results,
		"failed": failed,
	})
}
