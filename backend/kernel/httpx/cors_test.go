package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowsExactlyTheListedOrigins(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	h := CORS("https://app.example.test", "https://manager.example.test/")(ok)

	get := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/x", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := get("https://app.example.test"); rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.test" ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("listed origin not allowed: %v", rec.Header())
	}
	if rec := get("https://manager.example.test"); rec.Header().Get("Access-Control-Allow-Origin") != "https://manager.example.test" {
		t.Fatalf("trailing slash in the list must not matter: %v", rec.Header())
	}
	if rec := get("https://evil.example.test"); rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Code != 200 {
		t.Fatalf("unlisted origin: served, but with no CORS headers; got %d %v", rec.Code, rec.Header())
	}
	if rec := get("https://app.example.test:444"); rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("a different port is a different origin")
	}
	if rec := get(""); rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Vary") != "" {
		t.Fatal("no Origin, no CORS")
	}

	pre := httptest.NewRequest("OPTIONS", "/api/x", nil)
	pre.Header.Set("Origin", "https://app.example.test")
	pre.Header.Set("Access-Control-Request-Method", "PUT")
	pre.Header.Set("Access-Control-Request-Headers", "authorization, if-match, content-type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, pre)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Headers") != "authorization, if-match, content-type" ||
		rec.Header().Get("Access-Control-Allow-Methods") == "" || rec.Header().Get("Access-Control-Max-Age") != "600" {
		t.Fatalf("preflight = %d %v", rec.Code, rec.Header())
	}
	pre.Header.Set("Origin", "https://evil.example.test")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, pre)
	if rec.Code != 403 {
		t.Fatalf("preflight from an unlisted origin = %d, want 403", rec.Code)
	}
}
