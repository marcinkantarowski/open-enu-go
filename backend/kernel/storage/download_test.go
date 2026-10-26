package storage_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/storage"
)

func TestDownloadServesASignedKeyAndRefusesEverythingElse(t *testing.T) {
	signer := &storage.Signer{Secret: []byte("download-test-secret-at-least-32-bytes"), BaseURL: "http://api.test"}
	mem := &storage.Memory{Signer: signer}
	ctx := scope.WithTenant(context.Background(), "11111111-1111-1111-1111-111111111111")
	key, err := mem.Write(ctx, "attachment", "report.pdf", strings.NewReader("%PDF-1.4 hello"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	storage.Download{Storage: mem, Signer: signer}.Mount(mux)
	get := func(url string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
		return rec
	}
	url, _ := mem.TemporaryURL(ctx, key, time.Minute)
	rec := get(url)
	body, _ := io.ReadAll(rec.Body)
	if rec.Code != 200 || string(body) != "%PDF-1.4 hello" {
		t.Fatalf("signed download: %d %s", rec.Code, body)
	}
	h := rec.Header()
	if h.Get("Content-Type") != "application/pdf" || h.Get("Content-Disposition") != `attachment; filename="report.pdf"` || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers: %v", h)
	}
	if rec := get(strings.Replace(url, "sig=", "sig=0", 1)); rec.Code != 403 {
		t.Fatalf("edited signature: %d", rec.Code)
	}
	other := strings.Replace(url, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", 1)
	if rec := get(other); rec.Code != 403 {
		t.Fatalf("another tenant's key under this signature: %d", rec.Code)
	}
	if rec := get("http://api.test/files/" + key); rec.Code != 403 {
		t.Fatalf("no signature: %d", rec.Code)
	}
	expired := storage.Signer{Secret: signer.Secret, BaseURL: signer.BaseURL, Now: func() time.Time { return time.Now().Add(-2 * time.Hour) }}
	if rec := get(expired.Sign(key, time.Minute)); rec.Code != 403 {
		t.Fatalf("expired: %d", rec.Code)
	}
	gone := signer.Sign("11111111-1111-1111-1111-111111111111/attachment/nothing-here.pdf", time.Minute)
	if rec := get(gone); rec.Code != 404 {
		t.Fatalf("missing file: %d", rec.Code)
	}
}
