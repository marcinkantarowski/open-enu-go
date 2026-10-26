package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/platform"
)

// The dispatcher's contract, which later subcommands inherit: nothing is
// accepted that was not registered, and asking for nothing is an error too.
func TestRunRefusesWhatIsNotRegistered(t *testing.T) {
	cases := map[string]struct {
		args []string
		want int
	}{
		"no arguments":    {nil, 2},
		"unknown command": {[]string{"frobnicate"}, 2},
		"version":         {[]string{"version"}, 0},
		"help":            {[]string{"help"}, 0},
		"modules":         {[]string{"modules"}, 0},
	}
	for name, tc := range cases {
		if got := run(tc.args); got != tc.want {
			t.Errorf("%s: run(%v) = %d, want %d", name, tc.args, got, tc.want)
		}
	}
}

func TestDiscoveredModulesFormAValidRegistry(t *testing.T) {
	if _, err := registry(); err != nil {
		t.Fatalf("the generated registry does not validate: %v", err)
	}
}

func TestHandlerServesHealthModulesAndTheEnvelope(t *testing.T) {
	pinged := false
	reg, err := module.NewRegistry(module.Module{
		Manifest: module.Manifest{Name: "ping", Description: "test", Enabled: true},
		Mount: func(mux *http.ServeMux, svc *platform.Services) {
			mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
				pinged = svc.Storage != nil
				w.WriteHeader(204)
			})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	svc := platform.InMemory(log)
	if err := reg.LoadLocales(svc.I18n); err != nil {
		t.Fatal(err)
	}
	h, err := newHandler(reg, svc, log)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 || body["status"] != "ok" {
		t.Errorf("/health: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/ping", nil))
	if rec.Code != 204 || !pinged || rec.Header().Get("X-Request-Id") == "" {
		t.Errorf("module route: %d pinged=%v id=%q", rec.Code, pinged, rec.Header().Get("X-Request-Id"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/missing", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 404 || body["error"].(map[string]any)["code"] != "not_found" {
		t.Errorf("unmatched path must get the envelope: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHealthURLIsLoopbackForAnyListenAddress(t *testing.T) {
	for addr, want := range map[string]string{
		":8080":         "http://127.0.0.1:8080/health",
		"0.0.0.0:9000":  "http://127.0.0.1:9000/health",
		"10.0.0.5:8080": "http://10.0.0.5:8080/health",
	} {
		if got := healthURL(addr); got != want {
			t.Errorf("%s: %s, want %s", addr, got, want)
		}
	}
}
