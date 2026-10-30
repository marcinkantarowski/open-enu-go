package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type tooLargeReply struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details struct {
			MaxBytes int64 `json:"maxBytes"`
		} `json:"details"`
	} `json:"error"`
}

// decodeJSON is a handler as a module writes one: decode, and answer a 400
// when that fails - which MaxBody must turn into the 413.
var decodeJSON = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	var v map[string]any
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		WriteError(w, r, BadRequest(KeyBadRequest).WithCause(err))
		return
	}
	WriteJSON(w, 200, map[string]any{"keys": len(v)})
})

func bigJSON(n int) []byte {
	return []byte(`{"pad":"` + strings.Repeat("x", n) + `"}`)
}

func TestMaxBodyAnswers413InTheEnvelope(t *testing.T) {
	h := Chain(decodeJSON, MaxBody(DefaultMaxBody))
	for name, chunked := range map[string]bool{"content-length": false, "chunked": true} {
		var body io.Reader = bytes.NewReader(bigJSON(2 << 20))
		if chunked {
			body = io.MultiReader(body) // no Len: httptest leaves ContentLength -1
		}
		req := httptest.NewRequest("POST", "/api/x", body)
		if chunked {
			req.ContentLength = -1
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var got tooLargeReply
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 413 ||
			got.Error.Code != CodePayloadTooLarge || got.Error.Details.MaxBytes != DefaultMaxBody || got.Error.Message != string(KeyPayloadTooLarge) {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/x", bytes.NewReader(bigJSON(1000))))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"keys":1`) {
		t.Fatalf("a small body passes untouched: %d %s", rec.Code, rec.Body.String())
	}
}

// The upload route's own, larger limit: AllowBody inside MaxBody lets a body
// past the default through, and stops at its own limit.
func TestAllowBodyRaisesTheLimitForItsRoute(t *testing.T) {
	read := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, r.Body)
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			WriteError(w, r, BadRequest(KeyBadRequest))
			return
		}
		WriteJSON(w, 200, map[string]any{"read": n})
	})
	h := Chain(Chain(read, AllowBody(3<<20)), MaxBody(DefaultMaxBody))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/upload", bytes.NewReader(make([]byte, 2<<20))))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"read":2097152`) {
		t.Fatalf("2 MiB under a 3 MiB route limit: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/upload", bytes.NewReader(make([]byte, 4<<20))))
	var got tooLargeReply
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 413 || got.Error.Details.MaxBytes != 3<<20 {
		t.Fatalf("4 MiB over the route's own limit: %d %s", rec.Code, rec.Body.String())
	}
	// Exactly the limit is allowed.
	rec = httptest.NewRecorder()
	Chain(read, MaxBody(10)).ServeHTTP(rec, httptest.NewRequest("POST", "/", bytes.NewReader(make([]byte, 10))))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"read":10`) {
		t.Fatalf("exactly the limit: %d %s", rec.Code, rec.Body.String())
	}
}
