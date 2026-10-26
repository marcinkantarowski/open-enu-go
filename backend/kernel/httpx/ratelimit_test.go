package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-enu/kernel/httpx"
)

func TestRateLimitRefusesTheBurstPlusOneAndKeepsClientsApart(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := httpx.RateLimit(60, 3)(ok)
	hit := func(addr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/auth/login", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for i := range 3 {
		if rec := hit("10.0.0.1:1234"); rec.Code != 204 {
			t.Fatalf("request %d: %d", i+1, rec.Code)
		}
	}
	rec := hit("10.0.0.1:9999") // same host, another port: the same client
	if rec.Code != 429 || !strings.Contains(rec.Body.String(), `"code":"rate_limited"`) || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("fourth request: %d %s %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if rec := hit("10.0.0.2:1234"); rec.Code != 204 {
		t.Fatalf("another client is limited too: %d", rec.Code)
	}
	// Behind the edge, the address the edge appended is the client - and a
	// made-up address the client put in front of it changes nothing, or a
	// fresh one per attempt would switch the limiter off.
	for _, spoofed := range []string{"", "203.0.113.7, ", "198.51.100.9, "} {
		req := httptest.NewRequest("POST", "/api/auth/login", nil)
		req.RemoteAddr = "172.18.0.2:4000"
		req.Header.Set("X-Forwarded-For", spoofed+"10.0.0.1")
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 429 {
			t.Fatalf("forwarded client %q not recognised: %d", spoofed+"10.0.0.1", rec.Code)
		}
	}
}
