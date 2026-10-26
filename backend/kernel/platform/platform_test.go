package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/event"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"
)

func TestInMemoryIsCompleteAndReal(t *testing.T) {
	s := InMemory(nil)
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "DB (platform.WithDatabase)") {
		t.Fatalf("InMemory alone has no database; Validate says where it comes from: %v", err)
	}
	if got := s.I18n.Translate("pl", "kernel.error.not_found", nil); got != "Nie znaleziono." {
		t.Fatalf("kernel catalogue loaded: %q", got)
	}
	ct, err := s.Crypto.Encrypt("x", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Crypto.Decrypt(ct, "t1"); p != "x" {
		t.Fatal("crypto round trip")
	}
	key, err := s.Storage.Write(scope.WithTenant(context.Background(), "t1"), "demo", "a.txt", strings.NewReader("a"), "text/plain")
	if err != nil || !strings.HasPrefix(key, "t1/demo/") {
		t.Fatalf("storage: %s %v", key, err)
	}
	hub := s.Events.Hub.(*event.Recorder)
	if len(hub.Updates()) != 0 {
		t.Fatal("recorder reachable")
	}
}

func TestValidateNamesTheMissingService(t *testing.T) {
	s := InMemory(nil)
	s.Storage = nil
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "Storage") {
		t.Fatalf("got %v", err)
	}
	s = InMemory(nil)
	s.Events.Hub = nil
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "Events") {
		t.Fatalf("got %v", err)
	}
}

func TestWithDatabaseCompletesThePlatform(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil); err != nil {
		t.Fatal(err)
	}
	s := InMemory(nil)
	if err := WithDatabase(s, &scope.Pool{Pool: db.App}, outbox.Options{}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Audit.(audit.Postgres); !ok {
		t.Fatal("persistAudit switches the trail to Postgres")
	}
	s2 := InMemory(nil)
	_ = WithDatabase(s2, &scope.Pool{Pool: db.App}, outbox.Options{}, false)
	if _, ok := s2.Audit.(*audit.Recorder); !ok {
		t.Fatal("a test keeps its recorder")
	}
}
