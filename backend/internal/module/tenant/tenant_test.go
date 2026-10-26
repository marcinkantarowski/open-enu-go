package tenant_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/setup"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/tenant"
	"github.com/open-enu/app/internal/module/tenant/contract"
)

// Through the bus against a real Postgres: the module has no routes of its
// own, so its contract IS its surface.

type stack struct {
	ctx   context.Context
	svc   *platform.Services
	prov  contract.Provisioner
	read  contract.Reader
	setup *countingSetup
}

type countingSetup struct{ created []string }

func (c *countingSetup) OnTenantCreated(_ context.Context, id string) error {
	c.created = append(c.created, id)
	return nil
}
func (*countingSetup) SeedExamples(context.Context, string) error { return nil }

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil, migrate.Source{Name: "tenant", FS: tenant.Module.Migrations}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 2, PollInterval: 100 * time.Millisecond}, false); err != nil {
		t.Fatal(err)
	}
	cs := &countingSetup{}
	svc.Setup = setup.NewRunner(svc.Log, setup.Step{Module: "counting", Setup: cs})
	tenant.Module.Mount(http.NewServeMux(), svc)
	return &stack{ctx: ctx, svc: svc, prov: contract.Provisioner{Bus: svc.Bus}, read: contract.NewReader(svc), setup: cs}
}

func (s *stack) entries(action string) (out []audit.Entry) {
	for _, e := range s.svc.Audit.(*audit.Recorder).Entries() {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

func TestCreateDerivesTheSlugRunsSetupAndIsDescribedFromTheCache(t *testing.T) {
	s := newStack(t)
	info, err := s.prov.Create(s.ctx, contract.Create{Name: "Acme & Sons", DefaultLocale: "pl"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Slug != "acme-sons" || info.Status != contract.StatusPending || info.Plan != "free" || info.DefaultLocale != "pl" || info.Version != 1 {
		t.Fatalf("created: %+v", info)
	}
	if len(s.setup.created) != 1 || s.setup.created[0] != info.ID {
		t.Fatalf("setup ran for %v, want [%s]", s.setup.created, info.ID)
	}
	got, found, err := s.read.Describe(s.ctx, info.ID)
	if err != nil || !found || got.Name != "Acme & Sons" {
		t.Fatalf("describe: %+v %v %v", got, found, err)
	}
	bySlug, found, _ := s.read.BySlug(s.ctx, "acme-sons")
	if !found || bySlug.ID != info.ID {
		t.Fatalf("by slug: %+v %v", bySlug, found)
	}
	e := s.entries("tenant.created")
	if len(e) != 1 || !e[0].Succeeded || e[0].After["slug"] != "acme-sons" || e[0].Before != nil {
		t.Fatalf("audit: %+v", e)
	}
	if _, err := s.prov.Create(s.ctx, contract.Create{Name: "Acme!", Slug: "acme-sons"}); err == nil {
		t.Fatal("a taken slug was accepted")
	} else if he, ok := err.(*httpx.Error); !ok || he.Status != 409 {
		t.Fatalf("taken slug: %v", err)
	}
	if _, err := s.prov.Create(s.ctx, contract.Create{Name: "   "}); err == nil {
		t.Fatal("an empty name was accepted")
	}
	if _, err := s.prov.Create(s.ctx, contract.Create{Name: "X", Slug: "Not A Slug"}); err == nil {
		t.Fatal("a malformed slug was accepted")
	}
	if failed := s.entries("tenant.created"); len(failed) != 4 || failed[1].Succeeded {
		t.Fatalf("failures are in the trail too: %d entries", len(failed))
	}
}

func TestActivateIsIdempotentAndInvalidatesTheDescription(t *testing.T) {
	s := newStack(t)
	info, _ := s.prov.Create(s.ctx, contract.Create{Name: "Beta"})
	if _, _, err := s.read.Describe(s.ctx, info.ID); err != nil { // primes the cache
		t.Fatal(err)
	}
	if err := s.prov.Activate(s.ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.read.Describe(s.ctx, info.ID)
	if !got.Active() || got.ActivatedAt == nil || got.Version != 2 {
		t.Fatalf("after activate, the cache still says: %+v", got)
	}
	if err := s.prov.Activate(s.ctx, info.ID); err != nil {
		t.Fatal("second activate failed:", err)
	}
	if again, _, _ := s.read.Describe(s.ctx, info.ID); again.Version != 2 {
		t.Fatalf("second activate changed the row: %+v", again)
	}
	if err := s.prov.Suspend(s.ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	if got, _, _ = s.read.Describe(s.ctx, info.ID); got.Status != contract.StatusSuspended || got.Active() {
		t.Fatalf("suspended: %+v", got)
	}
	if err := s.prov.Activate(s.ctx, "0192f000-0000-7000-8000-0000000000ff"); err == nil {
		t.Fatal("an unknown tenant activated")
	}
}

func TestDeleteAnnouncesBeforeRemovingAndThePurgeTaskUsesIt(t *testing.T) {
	s := newStack(t)
	var heard []string
	s.svc.Outbox.Subscribe("tenant.deleted", func(_ context.Context, d outbox.Delivered) error {
		heard = append(heard, d.Payload()["slug"].(string))
		return nil
	})
	fresh, _ := s.prov.Create(s.ctx, contract.Create{Name: "Fresh"})
	stale, _ := s.prov.Create(s.ctx, contract.Create{Name: "Stale"})
	// Backdate the stale one past the window, as the schema owner.
	if _, err := s.svc.DB.Exec(s.ctx, `UPDATE tenant SET created_at = now() - interval '9 days' WHERE id = $1`, stale.ID); err != nil {
		t.Fatal(err)
	}
	would, err := tenant.PurgeUnverified(s.ctx, s.svc, 7, true)
	if err != nil || len(would) != 1 || would[0].ID != stale.ID {
		t.Fatalf("dry run: %+v %v", would, err)
	}
	if _, found, _ := s.read.Describe(s.ctx, stale.ID); !found {
		t.Fatal("a dry run deleted")
	}
	if _, err := tenant.PurgeUnverified(s.ctx, s.svc, 7, false); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := s.read.Describe(s.ctx, stale.ID); found {
		t.Fatal("the stale tenant survived the purge")
	}
	if _, found, _ := s.read.Describe(s.ctx, fresh.ID); !found {
		t.Fatal("the fresh tenant was purged")
	}
	done, cancel := s.svc.Outbox.Completed()
	defer cancel()
	if err := s.svc.Outbox.Start(s.ctx); err != nil {
		t.Fatal(err)
	}
	defer s.svc.Outbox.Stop(s.ctx)
	// Three events were queued: two tenant.created and the one deleted.
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatalf("event %d of 3 never delivered", i+1)
		}
	}
	if len(heard) != 1 || heard[0] != "stale" {
		t.Fatalf("heard %v", heard)
	}
	if e := s.entries("tenant.deleted"); len(e) != 1 || e[0].TenantID != stale.ID || e[0].After != nil {
		t.Fatalf("audit: %+v", e)
	}
}
