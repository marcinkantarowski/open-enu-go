package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/i18n"
)

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("not JSON: %s", body)
	}
	return m
}

func errorOf(t *testing.T, body []byte) map[string]any {
	t.Helper()
	e, ok := decode(t, body)["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error envelope in %s", body)
	}
	return e
}

func TestErrorEnvelopeShape(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, "req-1"))
	WriteError(rec, r, NotFound(i18n.Key("example.project.missing")))

	if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	e := errorOf(t, rec.Body.Bytes())
	if e["code"] != "not_found" || e["message"] != "example.project.missing" || e["requestId"] != "req-1" {
		t.Errorf("envelope = %v", e)
	}
	if _, has := e["details"]; has {
		t.Errorf("details should be omitted when empty: %v", e)
	}
}

func TestTranslatorOnTheContextResolvesTheMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r = r.WithContext(WithTranslator(r.Context(), func(_ context.Context, k i18n.Key) string { return "PL:" + string(k) }))
	WriteError(rec, r, Forbidden(KeyForbidden))
	if e := errorOf(t, rec.Body.Bytes()); e["message"] != "PL:kernel.error.forbidden" {
		t.Errorf("message = %v", e["message"])
	}
}

func TestConflictCarriesBothVersions(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", "/x", nil)
	WriteError(rec, r, Conflict(ConflictBody{Resource: "project", ResourceID: "p1", YourVersion: 3, CurrentVersion: 4, Current: map[string]any{"name": "new"}}))
	if rec.Code != 409 {
		t.Fatalf("status %d", rec.Code)
	}
	d := errorOf(t, rec.Body.Bytes())["details"].(map[string]any)
	if d["resource"] != "project" || d["resourceId"] != "p1" || d["yourVersion"] != float64(3) || d["currentVersion"] != float64(4) {
		t.Errorf("details = %v", d)
	}
	if d["current"].(map[string]any)["name"] != "new" {
		t.Errorf("current = %v", d["current"])
	}
}

func TestValidationFailedListsViolations(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest("POST", "/x", nil), ValidationFailed(map[string]i18n.Key{"name": "kernel.validation.required"}))
	e := errorOf(t, rec.Body.Bytes())
	if rec.Code != 422 || e["code"] != "validation_failed" {
		t.Fatalf("%d %v", rec.Code, e)
	}
	v := e["details"].(map[string]any)["violations"].(map[string]any)
	if v["name"] != "kernel.validation.required" {
		t.Errorf("violations = %v", v)
	}
}

func TestUnknownErrorIsA500ThatHidesTheCause(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, "req-9"))
	WriteError(rec, r, errors.New("pq: connection refused to 10.0.0.5"))

	e := errorOf(t, rec.Body.Bytes())
	if rec.Code != 500 || e["code"] != "internal_error" || e["message"] != string(KeyInternal) {
		t.Errorf("%d %v", rec.Code, e)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("cause leaked to the client: %s", rec.Body.String())
	}
	if !strings.Contains(buf.String(), "10.0.0.5") || !strings.Contains(buf.String(), "req-9") {
		t.Errorf("cause and request id must be in the log: %s", buf.String())
	}
}

func TestRequestIDKeptMintedAndEchoed(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = RequestIDFrom(r.Context()) }))

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(RequestIDHeader, "abc-123.X")
	h.ServeHTTP(rec, r)
	if seen != "abc-123.X" || rec.Header().Get(RequestIDHeader) != "abc-123.X" {
		t.Errorf("well-formed id not kept: ctx=%q header=%q", seen, rec.Header().Get(RequestIDHeader))
	}

	rec = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set(RequestIDHeader, "has spaces and <tags>")
	h.ServeHTTP(rec, r)
	if seen == "has spaces and <tags>" || len(seen) != 32 || rec.Header().Get(RequestIDHeader) != seen {
		t.Errorf("malformed id should be replaced by a minted one: ctx=%q header=%q", seen, rec.Header().Get(RequestIDHeader))
	}
}

func TestRecoverTurnsAPanicIntoAnEnvelope(t *testing.T) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	defer slog.SetDefault(prev)

	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("nil map write") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 500 || errorOf(t, rec.Body.Bytes())["code"] != "internal_error" {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestAccessLogHasTheGreppableFields(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/things/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := Chain(mux, RequestID, AccessLog(log))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/things/42", nil))

	line := decode(t, buf.Bytes())
	if line["route"] != "GET /api/things/{id}" || line["path"] != "/api/things/42" || line["status"] != float64(204) || line["request_id"] == "" {
		t.Errorf("log line = %v", line)
	}
}

func TestHealthShallowAndDeep(t *testing.T) {
	h := &Health{Stage: "dev", Kernel: "0.0.0", Checks: map[string]Check{
		"postgres": func(context.Context) error { return nil },
		"redis":    func(context.Context) error { return errors.New("dial tcp: refused") },
	}}
	mux := http.NewServeMux()
	h.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	if rec.Code != 200 || decode(t, rec.Body.Bytes())["status"] != "ok" {
		t.Errorf("shallow: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/health/deep", nil))
	body := decode(t, rec.Body.Bytes())
	if rec.Code != 503 || body["status"] != "degraded" {
		t.Fatalf("deep: %d %s", rec.Code, rec.Body.String())
	}
	if failed := body["failed"].([]any); len(failed) != 1 || failed[0] != "redis" {
		t.Errorf("failed = %v", failed)
	}
	redis := body["checks"].(map[string]any)["redis"].(map[string]any)
	// The driver's message stays in the log: the endpoint is public, and
	// that text names hosts and ports.
	if redis["ok"] != false || redis["error"] != "unavailable" || strings.Contains(rec.Body.String(), "refused") {
		t.Errorf("redis check = %v", redis)
	}
}

func TestHealthDeepTimesOutASlowCheck(t *testing.T) {
	h := &Health{Timeout: 20 * time.Millisecond, Checks: map[string]Check{
		"slow": func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
	}}
	mux := http.NewServeMux()
	h.Mount(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/health/deep", nil))
	if rec.Code != 503 {
		t.Errorf("a check that never answers must count as failed: %d", rec.Code)
	}
}

func TestNotFoundHandlerUsesTheEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/", NotFoundHandler())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/nope", nil))
	if rec.Code != 404 || errorOf(t, rec.Body.Bytes())["code"] != "not_found" {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestServerRunsAndDrains(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	s := &Server{Addr: "127.0.0.1:0", Handler: mux, Log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))}

	// Addr :0 means we cannot know the port; this test only proves Run honours
	// ctx and returns nil on a clean shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestChainTracesTheMatchedPatternWhenAsked(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "trace")
	t.Setenv(RouteTraceEnv, trace)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/things/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	h := Chain(mux, RequestID) // RequestID passes a copy of the request on
	for _, p := range []string{"/api/things/1", "/api/things/2", "/nowhere"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
	}
	got, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "GET /api/things/{id}\nGET /api/things/{id}\n" {
		t.Fatalf("trace:\n%s", got)
	}
}
