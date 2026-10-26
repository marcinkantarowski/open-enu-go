package progress_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/event"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	kprogress "github.com/open-enu/kernel/progress"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/progress"
)

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

// The reporter under test is the kernel's Postgres one - what the process
// runs - and the route reads what it wrote. The hub is a recorder, so the
// push is asserted beside the poll.
func TestAJobIsPersistedPushedAndReadByItsTenantOnly(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	hub := &event.Recorder{}
	pool := &scope.Pool{Pool: db.App}
	reporter := kprogress.Postgres{Store: kprogress.Store{Pool: pool}, Hub: hub, Log: svc.Log}
	svc.Progress = reporter
	if err := platform.WithDatabase(svc, pool, outbox.Options{MaxWorkers: 1, PollInterval: time.Second}, false); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	progress.Module.Mount(mux, svc)
	keys := auth.MemoryKeys{}
	h := httpx.Chain(mux, httpx.RequestID, httpx.Recover, auth.Middleware(svc.Signer, keys))
	get := func(id, authz string) (int, map[string]any) {
		req := httptest.NewRequest("GET", "/api/progress/"+id, nil)
		if authz != "" {
			req.Header.Set("Authorization", "Bearer "+authz)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}
	token := func(tenant string, perms ...string) string {
		tok, _ := svc.Signer.Sign(auth.Claims{Subject: "u", TenantID: tenant, Audience: auth.RealmTenant, Permissions: perms}, time.Hour)
		return tok
	}

	inA := scope.WithTenant(ctx, tenantA)
	id, err := reporter.Start(inA, "example.archive", 4, "Archiving")
	if err != nil || len(id) != 26 {
		t.Fatalf("start: %q %v", id, err)
	}
	if err := reporter.Advance(inA, id, 3, "Beta"); err != nil {
		t.Fatal(err)
	}
	code, body := get(id, token(tenantA, "progress.view"))
	if code != 200 || body["done"] != float64(3) || body["percent"] != float64(75) || body["status"] != "running" || body["label"] != "Beta" || body["finishedAt"] != nil {
		t.Fatalf("running: %d %v", code, body)
	}
	if code, _ := get(id, token(tenantB, "progress.view")); code != 404 {
		t.Fatalf("another tenant's job: %d", code)
	}
	if code, _ := get(id, token(tenantA, "example.view")); code != 403 {
		t.Fatalf("without progress.view: %d", code)
	}
	if code, _ := get(id, ""); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
	key := keys.Add("sk_progress-test", auth.Principal{Subject: "k", TenantID: tenantA, Realm: auth.RealmTenant, Permissions: []string{"progress.view"}})
	if code, _ := get(id, key); code != 200 {
		t.Fatalf("as a key: %d", code)
	}
	// Advancing past total clamps; finishing sets done = total.
	_ = reporter.Advance(inA, id, 5, "")
	if err := reporter.Finish(inA, id, map[string]any{"archived": 4}); err != nil {
		t.Fatal(err)
	}
	code, body = get(id, token(tenantA, "progress.view"))
	if code != 200 || body["done"] != float64(4) || body["percent"] != float64(100) || body["status"] != "done" || body["result"].(map[string]any)["archived"] != float64(4) || body["finishedAt"] == nil || body["label"] != "Beta" {
		t.Fatalf("done: %v", body)
	}
	// Finished is final: a late Fail changes nothing.
	_ = reporter.Fail(inA, id, "too late")
	if _, body = get(id, token(tenantA, "progress.view")); body["status"] != "done" {
		t.Fatalf("a finished job was failed: %v", body)
	}
	// Every write was pushed to the job's topic, in the route's shape.
	ups := hub.Updates()
	if len(ups) != 4 {
		t.Fatalf("%d updates, want 4 (start, advance, advance, finish)", len(ups))
	}
	var last map[string]any
	_ = json.Unmarshal(ups[3].Data, &last)
	if ups[3].Topic != kprogress.Topic(tenantA, id) || last["status"] != "done" || last["percent"] != float64(100) {
		t.Fatalf("last push: %s %s", ups[3].Topic, ups[3].Data)
	}
	// A failure.
	failed, _ := reporter.Start(inA, "example.archive", 2, "")
	_ = reporter.Fail(inA, failed, "disk full")
	if _, body = get(failed, token(tenantA, "progress.view")); body["status"] != "failed" || body["failureReason"] != "disk full" || body["label"] != nil {
		t.Fatalf("failed: %v", body)
	}
	// Outside a tenant the reporter only logs; nothing is stored.
	loose, err := reporter.Start(ctx, "system.sweep", 1, "")
	if err != nil || loose == "" {
		t.Fatal(err)
	}
	if _, found, _ := reporter.Get(scope.WithTenant(ctx, tenantA), loose); found {
		t.Fatal("a tenantless job was persisted")
	}
	if code, _ := get("not-a-job", token(tenantA, "progress.view")); code != 404 {
		t.Fatalf("unknown job: %d", code)
	}
}
