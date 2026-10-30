package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// probe asks mux for path from remote, with the headers given as name, value pairs.
func probe(mux http.Handler, path, remote string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = remote
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Add(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHealthShallowAndDeep(t *testing.T) {
	h := &Health{Stage: "dev", Kernel: "9.9.9", Checks: map[string]Check{
		"postgres": func(context.Context) error { return nil },
		"redis":    func(context.Context) error { return errors.New("dial tcp: refused") },
	}}
	mux := http.NewServeMux()
	h.Mount(mux)

	rec := probe(mux, "/health", "203.0.113.7:4000")
	if rec.Code != 200 || decode(t, rec.Body.Bytes())["status"] != "ok" || strings.Contains(rec.Body.String(), "9.9.9") {
		t.Errorf("shallow is public and names no version: %d %s", rec.Code, rec.Body.String())
	}

	rec = probe(mux, "/health/deep", "127.0.0.1:4000")
	body := decode(t, rec.Body.Bytes())
	if rec.Code != 503 || body["status"] != "degraded" {
		t.Fatalf("deep: %d %s", rec.Code, rec.Body.String())
	}
	if failed := body["failed"].([]any); len(failed) != 1 || failed[0] != "redis" {
		t.Errorf("failed = %v", failed)
	}
	redis := body["checks"].(map[string]any)["redis"].(map[string]any)
	// The driver's message stays in the log: that text names hosts and ports.
	if redis["ok"] != false || redis["error"] != "unavailable" || strings.Contains(rec.Body.String(), "refused") {
		t.Errorf("redis check = %v", redis)
	}
}

func TestHealthDeepAnswersOnlyTheTokenOrAPrivateNetwork(t *testing.T) {
	ran := 0
	h := &Health{Checks: map[string]Check{"postgres": func(context.Context) error { ran++; return nil }}}
	mux := http.NewServeMux()
	h.Mount(mux)

	for _, c := range []struct {
		name, remote string
		headers      []string
		want         int
	}{
		{"loopback", "127.0.0.1:1", nil, 200},
		{"a container on the private network", "172.18.0.5:1", nil, 200},
		{"a stranger", "203.0.113.7:1", nil, 404},
		{"a stranger through the edge", "172.18.0.2:1", []string{"X-Forwarded-For", "203.0.113.7"}, 404},
		{"a private hop through the edge", "172.18.0.2:1", []string{"X-Forwarded-For", "10.0.0.4"}, 200},
	} {
		if rec := probe(mux, "/health/deep", c.remote, c.headers...); rec.Code != c.want {
			t.Errorf("no token, %s: %d %s, want %d", c.name, rec.Code, rec.Body.String(), c.want)
		}
	}

	h.Token = "s3cret-health-token"
	before := ran
	for _, c := range []struct {
		name, remote, token string
		want                int
	}{
		{"the token from anywhere", "203.0.113.7:1", "s3cret-health-token", 200},
		{"loopback without it", "127.0.0.1:1", "", 404},
		{"a wrong token", "127.0.0.1:1", "s3cret-health-tokem", 404},
		{"a prefix of it", "203.0.113.7:1", "s3cret", 404},
	} {
		var headers []string
		if c.token != "" {
			headers = []string{HealthTokenHeader, c.token}
		}
		rec := probe(mux, "/health/deep", c.remote, headers...)
		if rec.Code != c.want {
			t.Errorf("token set, %s: %d %s, want %d", c.name, rec.Code, rec.Body.String(), c.want)
		}
		if c.want == 404 && errorOf(t, rec.Body.Bytes())["code"] != "not_found" {
			t.Errorf("token set, %s: a refusal must look like any missing route: %s", c.name, rec.Body.String())
		}
	}
	if ran != before+1 {
		t.Errorf("a refused probe must not touch the database: %d checks ran, want 1", ran-before)
	}
}

func TestHealthDeepTimesOutASlowCheck(t *testing.T) {
	h := &Health{Timeout: 20 * time.Millisecond, Checks: map[string]Check{
		"slow": func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
	}}
	mux := http.NewServeMux()
	h.Mount(mux)
	if rec := probe(mux, "/health/deep", "127.0.0.1:1"); rec.Code != 503 {
		t.Errorf("a check that never answers must count as failed: %d", rec.Code)
	}
}
