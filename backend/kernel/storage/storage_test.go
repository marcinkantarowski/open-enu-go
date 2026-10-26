package storage

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/scope"
)

func backends(t *testing.T) map[string]Storage {
	t.Helper()
	signer := &Signer{Secret: []byte("s"), BaseURL: "https://api.test"}
	return map[string]Storage{
		"memory": &Memory{Signer: signer},
		"local":  &Local{Root: t.TempDir(), Signer: signer},
	}
}

func TestEveryBackendScopesByTenant(t *testing.T) {
	for name, st := range backends(t) {
		t.Run(name, func(t *testing.T) {
			a := scope.WithTenant(context.Background(), "a")
			b := scope.WithTenant(context.Background(), "b")
			key, err := st.Write(a, "attachment", "../Quarterly Report (final).PDF", strings.NewReader("pdf bytes"), "application/pdf")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(key, "a/attachment/") || !strings.HasSuffix(key, "-Quarterly_Report_final_.PDF") {
				t.Fatalf("key shape: %s", key)
			}
			rc, err := st.Open(a, key)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			if string(data) != "pdf bytes" {
				t.Fatalf("read back %q", data)
			}
			if info, err := st.Stat(a, key); err != nil || info.Size != 9 || info.ContentType != "application/pdf" {
				t.Fatalf("stat %+v %v", info, err)
			}
			for _, bad := range []context.Context{b, context.Background()} {
				if _, err := st.Open(bad, key); !errors.Is(err, ErrNotFound) {
					t.Fatalf("another tenant's read is a not-found, got %v", err)
				}
				if err := st.Delete(bad, key); !errors.Is(err, ErrNotFound) {
					t.Fatalf("another tenant's delete is a not-found, got %v", err)
				}
				if _, err := st.TemporaryURL(bad, key, time.Minute); !errors.Is(err, ErrNotFound) {
					t.Fatal("another tenant cannot sign a URL for it")
				}
			}
			if _, err := st.Open(a, "a/attachment/../../etc/passwd"); !errors.Is(err, ErrNotFound) {
				t.Fatal("traversal is a not-found")
			}
			u, err := st.TemporaryURL(a, key, time.Minute)
			if err != nil || !strings.HasPrefix(u, "https://api.test/files/a/attachment/") || !strings.Contains(u, "sig=") {
				t.Fatalf("url %s %v", u, err)
			}
			if err := st.Delete(a, key); err != nil {
				t.Fatal(err)
			}
			if _, err := st.Stat(a, key); !errors.Is(err, ErrNotFound) {
				t.Fatal("gone after delete")
			}
			if _, err := st.Write(a, "Not-A-Module", "x", strings.NewReader(""), "text/plain"); err == nil {
				t.Fatal("module name shape")
			}
			gk, _ := st.Write(context.Background(), "settings", "", strings.NewReader("g"), "text/plain")
			if !strings.HasPrefix(gk, "global/settings/") || !strings.HasSuffix(gk, "-file") {
				t.Fatalf("unscoped writes go under global with a default name: %s", gk)
			}
		})
	}
}

func TestSignerRefusesEditsAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	s := Signer{Secret: []byte("secret"), BaseURL: "https://api.test/", Now: func() time.Time { return now }}
	u, _ := url.Parse(s.Sign("a/attachment/x-y.pdf", 5*time.Minute))
	if u.Path != "/files/a/attachment/x-y.pdf" {
		t.Fatal(u.Path)
	}
	q := u.Query()
	if err := s.Verify("a/attachment/x-y.pdf", q.Get("expires"), q.Get("sig")); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify("a/attachment/other.pdf", q.Get("expires"), q.Get("sig")); !errors.Is(err, ErrBadSignature) {
		t.Fatal("another key")
	}
	if err := s.Verify("a/attachment/x-y.pdf", "9999999999", q.Get("sig")); !errors.Is(err, ErrBadSignature) {
		t.Fatal("edited expiry")
	}
	if err := s.Verify("a/attachment/x-y.pdf", q.Get("expires"), "00"); !errors.Is(err, ErrBadSignature) {
		t.Fatal("edited signature")
	}
	now = now.Add(6 * time.Minute)
	if err := s.Verify("a/attachment/x-y.pdf", q.Get("expires"), q.Get("sig")); !errors.Is(err, ErrBadSignature) {
		t.Fatal("expired")
	}
	if err := s.Verify("k", "soon", "x"); !errors.Is(err, ErrBadSignature) {
		t.Fatal("garbage expiry")
	}
}
