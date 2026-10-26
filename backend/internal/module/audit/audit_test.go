package audit_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	kaudit "github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/audit"
	"github.com/open-enu/app/internal/module/audit/contract"
)

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

func newStack(t *testing.T) (*platform.Services, http.Handler, auth.MemoryKeys) {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.Actor = auth.Actor{}
	// The trail in Postgres, as the process has it: the module reads what
	// the kernel wrote.
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 1, PollInterval: time.Second}, true); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	audit.Module.Mount(mux, svc)
	keys := auth.MemoryKeys{}
	return svc, httpx.Chain(mux, httpx.RequestID, httpx.Recover, auth.Middleware(svc.Signer, keys)), keys
}

func get(h http.Handler, path, authz string) (int, map[string]any) {
	req := httptest.NewRequest("GET", path, nil)
	if authz != "" {
		req.Header.Set("Authorization", "Bearer "+authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestATenantReadsItsOwnTrailNewestFirstWithinTheLimit(t *testing.T) {
	svc, h, keys := newStack(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		svc.Audit.Record(ctx, kaudit.Entry{Action: "demo.thing.changed", TenantID: tenantA, ActorID: "u1", SubjectID: "s1", Succeeded: true,
			Before: map[string]any{"n": i}, After: map[string]any{"n": i + 1}, At: time.Now().Add(time.Duration(i) * time.Second)})
	}
	svc.Audit.Record(ctx, kaudit.Entry{Action: "demo.other", TenantID: tenantB, ActorID: "u2", Succeeded: false, FailureReason: "boom"})
	svc.Audit.Record(ctx, kaudit.Entry{Action: "system.sweep", Succeeded: true}) // no tenant at all
	owner, _ := svc.Signer.Sign(auth.Claims{Subject: "u1", TenantID: tenantA, Audience: auth.RealmTenant, Permissions: []string{"audit.view"}}, time.Hour)
	code, body := get(h, "/api/audit", owner)
	items, _ := body["items"].([]any)
	if code != 200 || len(items) != 3 {
		t.Fatalf("A: %d %v", code, body)
	}
	first := items[0].(map[string]any)
	if first["after"].(map[string]any)["n"] != float64(3) || first["actorId"] != "u1" || first["tenantId"] != tenantA || first["succeeded"] != true || first["onBehalfOfId"] != nil {
		t.Fatalf("newest first, full shape: %v", first)
	}
	if code, body := get(h, "/api/audit?limit=2", owner); code != 200 || len(body["items"].([]any)) != 2 {
		t.Fatalf("limit: %d %v", code, body)
	}
	if code, body := get(h, "/api/audit?limit=500", owner); code != 200 || len(body["items"].([]any)) != 3 {
		t.Fatalf("limit over the maximum falls back to the default: %d %v", code, body)
	}
	other, _ := svc.Signer.Sign(auth.Claims{Subject: "u2", TenantID: tenantB, Audience: auth.RealmTenant, Permissions: []string{"audit.view"}}, time.Hour)
	code, body = get(h, "/api/audit", other)
	items, _ = body["items"].([]any)
	if code != 200 || len(items) != 1 || items[0].(map[string]any)["failureReason"] != "boom" {
		t.Fatalf("B: %d %v", code, body)
	}
	key := keys.Add("sk_audit-test", auth.Principal{Subject: "k", TenantID: tenantA, Realm: auth.RealmTenant, Permissions: []string{"audit.view"}})
	if code, body := get(h, "/api/audit", key); code != 200 || len(body["items"].([]any)) != 3 {
		t.Fatalf("as a key: %d %v", code, body)
	}
	if code, _ := get(h, "/api/audit", ""); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
	member, _ := svc.Signer.Sign(auth.Claims{Subject: "u3", TenantID: tenantA, Audience: auth.RealmTenant, Permissions: []string{"example.view"}}, time.Hour)
	if code, _ := get(h, "/api/audit", member); code != 403 {
		t.Fatalf("without audit.view: %d", code)
	}
	// The operator's read: every tenant, and the system's own entries.
	all, err := contract.NewReader(svc).Recent(ctx, nil, 10)
	if err != nil || len(all) != 5 {
		t.Fatalf("unscoped: %d %v", len(all), err)
	}
}

func TestGdprExportsActionsAndErasureAnonymisesWithoutDeleting(t *testing.T) {
	svc, _, _ := newStack(t)
	ctx := context.Background()
	svc.Audit.Record(ctx, kaudit.Entry{Action: "demo.one", TenantID: tenantA, ActorID: "person", Succeeded: true, Before: map[string]any{"name": "Jane"}})
	svc.Audit.Record(ctx, kaudit.Entry{Action: "demo.two", TenantID: tenantB, ActorID: "op", OnBehalfOf: "person", Succeeded: true})
	svc.Audit.Record(ctx, kaudit.Entry{Action: "demo.three", TenantID: tenantA, ActorID: "someone-else", Succeeded: true})
	g := audit.Module.Gdpr(svc)
	export, err := g.ExportFor(ctx, "person")
	if err != nil || len(export["actions"]) != 2 || export["actions"][0]["action"] != "demo.one" {
		t.Fatalf("export: %v %v", export, err)
	}
	n, err := g.EraseFor(ctx, "person")
	if err != nil || n != 2 {
		t.Fatalf("erase: %d %v", n, err)
	}
	all, _ := contract.NewReader(svc).Recent(ctx, nil, 10)
	var erased, kept int
	for _, e := range all {
		switch e.Action {
		case "demo.one":
			if *e.ActorID != "erased" || e.Before != nil {
				t.Fatalf("not anonymised: %+v", e)
			}
			erased++
		case "demo.two":
			if *e.OnBehalfOfID != "erased" || *e.ActorID != "op" {
				t.Fatalf("on-behalf-of not anonymised: %+v", e)
			}
			erased++
		case "demo.three":
			kept++
		}
	}
	if erased != 2 || kept != 1 {
		t.Fatalf("erased %d kept %d of %d", erased, kept, len(all))
	}
	if export, _ := g.ExportFor(ctx, "person"); len(export["actions"]) != 0 {
		t.Fatalf("still exportable after erasure: %v", export)
	}
}
