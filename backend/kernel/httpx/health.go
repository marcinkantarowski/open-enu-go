package httpx

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
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
//	              nothing. Public, and says nothing worth knowing to a
//	              stranger: no version.
//	/health/deep  readiness - runs every registered Check with a short timeout
//	              and answers 503 naming what failed. Not public: each call
//	              pings the database, so it answers only a request carrying
//	              X-Health-Token equal to Token or, with no Token set, one from
//	              a loopback or private address with no public hop in
//	              X-Forwarded-For. Anyone else gets the 404 of a route that
//	              does not exist.
type Health struct {
	Stage   string
	Kernel  string
	Token   string // HEALTH_TOKEN; empty means private networks only
	Checks  map[string]Check
	Timeout time.Duration // per check; 2s when zero
}

// HealthTokenHeader carries the token an external monitor sends to /health/deep.
const HealthTokenHeader = "X-Health-Token"

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
	})
}

// admits decides who may run the deep checks: the token holder, or - only
// when no token is configured - a caller on a private network. Behind the
// edge every RemoteAddr is the edge's own private one, so the hops the edge
// recorded in X-Forwarded-For have to be private too.
func (h *Health) admits(r *http.Request) bool {
	if h.Token != "" {
		got := r.Header.Get(HealthTokenHeader)
		return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(h.Token)) == 1
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	hops := []string{host}
	for _, xff := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(xff, ",")...)
	}
	for _, hop := range hops {
		ip := net.ParseIP(strings.TrimSpace(hop))
		if ip == nil || !(ip.IsLoopback() || ip.IsPrivate()) {
			return false
		}
	}
	return true
}

type checkResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Ms    int64  `json:"ms"`
}

func (h *Health) deep(w http.ResponseWriter, r *http.Request) {
	if !h.admits(r) {
		WriteError(w, r, NotFound(KeyNotFound))
		return
	}
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
			// Do not lean on the token: the driver's message names hosts,
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
