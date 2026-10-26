package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-enu/kernel/i18n"
)

func TestLocaleMiddlewareTranslatesTheEnvelope(t *testing.T) {
	cat, _ := i18n.NewCatalogue("en", "en", "pl")
	if err := cat.Load("kernel", i18n.KernelLocales()); err != nil {
		t.Fatal(err)
	}
	notFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, NotFound(KeyNotFound))
	})
	stored := func(r *http.Request) string { return r.Header.Get("X-Test-Pref") }
	h := Chain(notFound, Locale(cat, stored))

	get := func(accept, pref string) (string, string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Accept-Language", accept)
		req.Header.Set("X-Test-Pref", pref)
		h.ServeHTTP(rec, req)
		return rec.Header().Get("Content-Language"), rec.Body.String()
	}
	if lang, body := get("pl-PL,pl;q=0.9", ""); lang != "pl" || !strings.Contains(body, "Nie znaleziono.") {
		t.Fatalf("Accept-Language: %s %s", lang, body)
	}
	if lang, body := get("de", ""); lang != "en" || !strings.Contains(body, `"message":"Not found."`) {
		t.Fatalf("default: %s %s", lang, body)
	}
	if lang, _ := get("en", "pl"); lang != "pl" {
		t.Fatal("a stored preference beats the header")
	}
	if lang, _ := get("pl", "de"); lang != "pl" {
		t.Fatal("a preference for a locale that does not ship is skipped")
	}
}
