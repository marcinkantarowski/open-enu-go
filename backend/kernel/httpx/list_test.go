package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPaginationClampsAndDefaults(t *testing.T) {
	page, size := 3, 500
	sort := "name"
	p := Paginate(&page, &size, Deref(&sort), "DESC")
	if p.Page != 3 || p.Size != 100 || p.Sort != "name" || p.Direction != "desc" || p.Offset() != 200 {
		t.Fatalf("%+v", p)
	}
	page, size = -1, 0
	p = Paginate(&page, &size, Deref[string](nil), "")
	if p.Page != 1 || p.Size != 25 || p.Sort != "" || p.Direction != "asc" {
		t.Fatalf("defaults: %+v", p)
	}
	if m := p.Meta(51); m.Pages != 3 || m.Total != 51 || m.Size != 25 {
		t.Fatalf("meta: %+v", m)
	}
}

func TestIfMatchReadsTheStrongETagOnly(t *testing.T) {
	for in, want := range map[string]int{`"7"`: 7, ` "12" `: 12} {
		if v, ok := IfMatch(in); !ok || v != want {
			t.Errorf("%q = %d %v", in, v, ok)
		}
	}
	for _, in := range []string{"", `"abc"`, `*`, `"1", "2"`, `W/"7"`, `7`} {
		if _, ok := IfMatch(in); ok {
			t.Errorf("%q accepted", in)
		}
	}
	if ETag(3) != `"3"` {
		t.Fatal(ETag(3))
	}
}

// A present If-Match that is not a strong ETag is a 400 naming the header,
// never "no lock"; absent and `*` reach the handler.
func TestValidIfMatchRefusesWhatIfMatchCannotRead(t *testing.T) {
	h := ValidIfMatch(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, in := range []string{`W/"3"`, `3`, `"abc"`, `"1", "2"`, `"3`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PATCH", "/x", nil)
		req.Header.Set("If-Match", in)
		h.ServeHTTP(rec, req)
		var body struct {
			Error struct {
				Code    string `json:"code"`
				Details struct {
					Violations map[string]string `json:"violations"`
				} `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 400 || body.Error.Code != CodeBadRequest ||
			body.Error.Details.Violations["If-Match"] != string(KeyIfMatchInvalid) {
			t.Errorf("%q: %d %s", in, rec.Code, rec.Body.String())
		}
	}
	for _, in := range []string{"", `"3"`, `*`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PATCH", "/x", nil)
		if in != "" {
			req.Header.Set("If-Match", in)
		}
		h.ServeHTTP(rec, req)
		if rec.Code != 204 || rec.Body.Len() != 0 {
			t.Errorf("%q must pass: %d %s", in, rec.Code, rec.Body.String())
		}
	}
}
