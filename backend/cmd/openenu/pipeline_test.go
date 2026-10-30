package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/heartbeat"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"
)

// echoPipeline is newHandler around one module route that decodes JSON the
// way a generated strict server does, and answers 400 when it cannot.
func echoPipeline(t *testing.T) http.Handler {
	t.Helper()
	reg, err := module.NewRegistry(module.Module{
		Manifest: module.Manifest{Name: "echo", Description: "test", Enabled: true},
		Mount: func(mux *http.ServeMux, svc *platform.Services) {
			mux.HandleFunc("POST /api/echo", func(w http.ResponseWriter, r *http.Request) {
				var v map[string]any
				if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
					httpx.WriteError(w, r, httpx.BadRequest(httpx.KeyBadRequest).WithCause(err))
					return
				}
				httpx.WriteJSON(w, 200, map[string]any{"ifMatch": r.Header.Get("If-Match"), "fields": len(v)})
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := platform.InMemory(log)
	if err := reg.LoadLocales(svc.I18n); err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(reg, svc, log)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

type envelopeReply struct {
	Error struct {
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		Details   map[string]any `json:"details"`
		RequestID string         `json:"requestId"`
	} `json:"error"`
}

func TestTheAPIRefusesABodyOverOneMiBWith413InTheEnvelope(t *testing.T) {
	h := echoPipeline(t)
	big := `{"pad":"` + strings.Repeat("x", 2<<20) + `"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/echo", strings.NewReader(big)))
	var got envelopeReply
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 413 || got.Error.Code != "payload_too_large" ||
		got.Error.Details["maxBytes"] != float64(httpx.DefaultMaxBody) || got.Error.RequestID == "" || got.Error.Message != "The request is too large." {
		t.Fatalf("2 MiB JSON: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/echo", strings.NewReader(`{"a":1}`)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"fields":1`) {
		t.Fatalf("a small body: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTheAPIRefusesAMalformedIfMatchAndPassesAStrongOne(t *testing.T) {
	h := echoPipeline(t)
	for _, in := range []string{`W/"3"`, `3`} {
		req := httptest.NewRequest("POST", "/api/echo", strings.NewReader(`{}`))
		req.Header.Set("If-Match", in)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var got envelopeReply
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 400 {
			t.Fatalf("%q: %d %s", in, rec.Code, rec.Body.String())
		}
		v, _ := got.Error.Details["violations"].(map[string]any)
		if v["If-Match"] != string(httpx.KeyIfMatchInvalid) {
			t.Fatalf("%q must name the header: %s", in, rec.Body.String())
		}
	}
	req := httptest.NewRequest("POST", "/api/echo", strings.NewReader(`{}`))
	req.Header.Set("If-Match", `"3"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ifMatch":"\"3\""`) {
		t.Fatalf("a strong ETag passes: %d %s", rec.Code, rec.Body.String())
	}
}

// `openenu health -heartbeat`: the worker's and the scheduler's healthcheck.
func TestHealthHeartbeatFreshStaleMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beat")
	if got := runHealth([]string{"-heartbeat", path}); got != 1 {
		t.Fatalf("missing file: exit %d, want 1", got)
	}
	if err := heartbeat.Touch(path); err != nil {
		t.Fatal(err)
	}
	if got := runHealth([]string{"-heartbeat", path}); got != 0 {
		t.Fatalf("fresh file: exit %d, want 0", got)
	}
	old := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if got := runHealth([]string{"-heartbeat", path}); got != 1 {
		t.Fatalf("stale file: exit %d, want 1", got)
	}
	if got := runHealth([]string{"-heartbeat", path, "-max-age", "5m"}); got != 0 {
		t.Fatalf("within a longer -max-age: exit %d, want 0", got)
	}
}

// What the line says, not only the code.
func TestCheckHeartbeatPrintsOneLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "beat")
	if err := heartbeat.Touch(path); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if checkHeartbeat(path, time.Minute, time.Now()) != 0 {
			t.Error("fresh must be 0")
		}
	})
	if !strings.HasPrefix(out, "ok: "+path+" beat ") || strings.Count(out, "\n") != 1 {
		t.Fatalf("output: %q", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = orig
	w.Close()
	var b bytes.Buffer
	_, _ = io.Copy(&b, r)
	return b.String()
}
