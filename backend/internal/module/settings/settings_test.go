package settings_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/settings"
	"github.com/open-enu/app/internal/module/tenant"
	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// Two declared flags stand in for every module's: one a tenant may set,
// one only an operator may. The resolver under test is the kernel's, over
// this module's store - what a `svc.Flags.Enabled` call anywhere gets.
var declared = []flags.Definition{
	{Identifier: "demo.archive", Name: "Bulk archive", Default: false, TenantEditable: true, Category: "demo"},
	{Identifier: "demo.exports", Name: "Exports", Description: "A platform kill switch.", Default: true},
	{Identifier: "demo.batch", Name: "Batch size", Type: flags.TypeInt, Default: 100, TenantEditable: true},
}

type stack struct {
	t    *testing.T
	ctx  context.Context
	svc  *platform.Services
	h    http.Handler
	a, b string
}

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil,
		migrate.Source{Name: "tenant", FS: tenant.Module.Migrations},
		migrate.Source{Name: "settings", FS: settings.Module.Migrations}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.Actor = auth.Actor{}
	svc.FlagDefinitions = declared
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 1, PollInterval: time.Second}, false); err != nil {
		t.Fatal(err)
	}
	resolver, err := flags.NewResolver(settings.Module.FlagStore(svc), declared...)
	if err != nil {
		t.Fatal(err)
	}
	svc.Flags = resolver
	mux := http.NewServeMux()
	tenant.Module.Mount(mux, svc)
	settings.Module.Mount(mux, svc)
	prov := tenantc.Provisioner{Bus: svc.Bus}
	mk := func(name string) string {
		info, err := prov.Create(ctx, tenantc.Create{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return info.ID
	}
	return &stack{t: t, ctx: ctx, svc: svc, a: mk("A"), b: mk("B"), h: httpx.Chain(mux, httpx.RequestID, httpx.Recover, auth.Middleware(svc.Signer, nil))}
}

type reply struct {
	code int
	raw  []byte
	body map[string]any
}

func (s *stack) do(method, path, authz string, payload any) reply {
	s.t.Helper()
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	if authz != "" {
		req.Header.Set("Authorization", "Bearer "+authz)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	r := reply{code: rec.Code, raw: rec.Body.Bytes()}
	_ = json.Unmarshal(r.raw, &r.body)
	return r
}

func (s *stack) member(tenant string, perms ...string) string {
	tok, _ := s.svc.Signer.Sign(auth.Claims{Subject: "u", TenantID: tenant, Audience: auth.RealmTenant, Permissions: perms}, time.Hour)
	return tok
}

func (s *stack) operator() string {
	tok, _ := s.svc.Signer.Sign(auth.Claims{Subject: "op", Audience: auth.RealmManager}, time.Hour)
	return tok
}

func items(r reply) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, it := range r.body["items"].([]any) {
		m := it.(map[string]any)
		out[m["identifier"].(string)] = m
	}
	return out
}

func TestATenantSeesEveryDeclarationAndMayChangeOnlyWhatIsEditable(t *testing.T) {
	s := newStack(t)
	owner := s.member(s.a, "settings.view", "settings.manage")
	r := s.do("GET", "/api/settings", owner, nil)
	if r.code != 200 || len(r.body["items"].([]any)) != 3 {
		t.Fatalf("list: %d %s", r.code, r.raw)
	}
	all := items(r)
	if all["demo.archive"]["effectiveValue"] != false || all["demo.archive"]["overridden"] != false || all["demo.exports"]["tenantEditable"] != false || all["demo.batch"]["type"] != "int" || all["demo.archive"]["id"] != "demo.archive" {
		t.Fatalf("shape: %s", r.raw)
	}
	if p := s.do("PUT", "/api/settings/demo.archive", owner, map[string]any{"value": true}); p.code != 200 || p.body["value"] != true || p.body["identifier"] != "demo.archive" {
		t.Fatalf("set: %d %s", p.code, p.raw)
	}
	if p := s.do("PUT", "/api/settings/demo.exports", owner, map[string]any{"value": true}); p.code != 400 || !strings.Contains(string(p.raw), "platform_managed") {
		t.Fatalf("a kill switch set by a tenant: %d %s", p.code, p.raw)
	}
	if p := s.do("PUT", "/api/settings/demo.nothing", owner, map[string]any{"value": true}); p.code != 404 {
		t.Fatalf("unknown: %d", p.code)
	}
	if p := s.do("PUT", "/api/settings/demo.archive", s.member(s.a, "settings.view"), map[string]any{"value": true}); p.code != 403 {
		t.Fatalf("without manage: %d", p.code)
	}
	if p := s.do("GET", "/api/settings", "", nil); p.code != 401 {
		t.Fatalf("anonymous: %d", p.code)
	}
	all = items(s.do("GET", "/api/settings", owner, nil))
	if all["demo.archive"]["effectiveValue"] != true || all["demo.archive"]["overridden"] != true {
		t.Fatalf("after: %v", all["demo.archive"])
	}
	// B inherits the default; the resolver agrees with the API for both.
	if other := items(s.do("GET", "/api/settings", s.member(s.b, "settings.view"), nil)); other["demo.archive"]["overridden"] != false {
		t.Fatalf("B sees A's override: %v", other["demo.archive"])
	}
	if !s.svc.Flags.Enabled(scope.WithTenant(s.ctx, s.a), "demo.archive", false) || s.svc.Flags.Enabled(scope.WithTenant(s.ctx, s.b), "demo.archive", false) || s.svc.Flags.Enabled(s.ctx, "demo.archive", false) {
		t.Fatal("the resolver disagrees with the API")
	}
	if e := entries(s, "settings.flag.set"); len(e) < 1 || e[0].TenantID != s.a || e[0].SubjectID != "demo.archive" || e[0].After["value"] != true {
		t.Fatalf("audit: %+v", e)
	}
}

func TestAnOperatorOverridesAnythingForOneTenantAndItBitesOnTheNextRead(t *testing.T) {
	s := newStack(t)
	op := s.operator()
	l := s.do("GET", "/api/manager/settings", op, nil)
	if l.code != 200 || len(l.body["items"].([]any)) != 3 {
		t.Fatalf("catalogue: %d %s", l.code, l.raw)
	}
	if l := s.do("GET", "/api/manager/settings", s.member(s.a, "settings.view", "settings.manage"), nil); l.code != 401 {
		t.Fatalf("a tenant token on the operator route: %d", l.code)
	}
	inA, inB := scope.WithTenant(s.ctx, s.a), scope.WithTenant(s.ctx, s.b)
	// Warm the cache with the default, then flip: the switch must bite at once.
	if !s.svc.Flags.Enabled(inA, "demo.exports", false) {
		t.Fatal("default")
	}
	if p := s.do("PUT", "/api/manager/tenants/"+s.a+"/settings/demo.exports", op, map[string]any{"value": false}); p.code != 200 || p.body["value"] != false {
		t.Fatalf("operator set: %d %s", p.code, p.raw)
	}
	if s.svc.Flags.Enabled(inA, "demo.exports", true) {
		t.Fatal("the kill switch did not bite on the next read")
	}
	if !s.svc.Flags.Enabled(inB, "demo.exports", false) {
		t.Fatal("A's switch reached B")
	}
	// The tenant sees it, cannot undo it, and the override stays a value - false, not unset.
	owner := s.member(s.a, "settings.view", "settings.manage")
	all := items(s.do("GET", "/api/settings", owner, nil))
	if all["demo.exports"]["effectiveValue"] != false || all["demo.exports"]["overridden"] != true {
		t.Fatalf("tenant view: %v", all["demo.exports"])
	}
	if p := s.do("PUT", "/api/settings/demo.exports", owner, map[string]any{"value": true}); p.code != 400 {
		t.Fatalf("tenant undoing a kill switch: %d", p.code)
	}
	// Other types resolve too; an unknown tenant or setting is 404.
	s.do("PUT", "/api/manager/tenants/"+s.a+"/settings/demo.batch", op, map[string]any{"value": 7})
	if n := s.svc.Flags.Int(inA, "demo.batch", 0); n != 7 {
		t.Fatalf("int override: %d", n)
	}
	if p := s.do("PUT", "/api/manager/tenants/00000000-0000-0000-0000-000000000000/settings/demo.batch", op, map[string]any{"value": 1}); p.code != 404 {
		t.Fatalf("unknown tenant: %d", p.code)
	}
	if p := s.do("PUT", "/api/manager/tenants/"+s.a+"/settings/nope.x", op, map[string]any{"value": 1}); p.code != 404 {
		t.Fatalf("unknown setting: %d", p.code)
	}
	// Setting it twice updates the one row; the trail has before and after.
	s.do("PUT", "/api/manager/tenants/"+s.a+"/settings/demo.exports", op, map[string]any{"value": true})
	e := entries(s, "settings.flag.set")
	last := e[len(e)-1]
	if last.Before["value"] != false || last.After["value"] != true || last.TenantID != s.a || last.ActorID != "op" {
		t.Fatalf("audit of an update: %+v", last)
	}
}

func entries(s *stack, action string) (out []audit.Entry) {
	for _, e := range s.svc.Audit.(*audit.Recorder).Entries() {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}
