package api_key_test

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
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/api_key"
	"github.com/open-enu/app/internal/module/tenant"
	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// The resolver under test is the one the middleware uses, so a key minted
// through POST authenticates the very next request - the loop closes here.

type stack struct {
	t      *testing.T
	ctx    context.Context
	db     *testdb.DB
	svc    *platform.Services
	h      http.Handler
	a, b   string // active tenants
	signer auth.Signer
}

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil,
		migrate.Source{Name: "tenant", FS: tenant.Module.Migrations},
		migrate.Source{Name: "api_key", FS: api_key.Module.Migrations}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.Actor = auth.Actor{}
	svc.Permissions = append(append([]string{}, tenant.Module.Permissions...), api_key.Module.Permissions...)
	svc.Permissions = append(svc.Permissions, "example.view", "example.manage")
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 1, PollInterval: 100 * time.Millisecond}, false); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	tenant.Module.Mount(mux, svc)
	api_key.Module.Mount(mux, svc)
	prov := tenantc.Provisioner{Bus: svc.Bus}
	mk := func(name string) string {
		info, err := prov.Create(ctx, tenantc.Create{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		if err := prov.Activate(ctx, info.ID); err != nil {
			t.Fatal(err)
		}
		return info.ID
	}
	keys, err := api_key.Module.Keys(svc), error(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &stack{t: t, ctx: ctx, db: db, svc: svc, a: mk("A"), b: mk("B"), signer: svc.Signer,
		h: httpx.Chain(mux, httpx.RequestID, httpx.Recover, auth.Middleware(svc.Signer, keys))}
}

type reply struct {
	code int
	body map[string]any
}

func (r reply) str(path ...string) string {
	var cur any = r.body
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[k]
	}
	s, _ := cur.(string)
	return s
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
	r := reply{code: rec.Code}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &r.body); err != nil {
			s.t.Fatalf("%s %s: not JSON: %s", method, path, rec.Body.String())
		}
	}
	return r
}

func (s *stack) owner(tenant string) string {
	tok, err := s.signer.Sign(auth.Claims{Subject: "user-1", TenantID: tenant, Audience: auth.RealmTenant, Permissions: s.svc.Permissions}, time.Hour)
	if err != nil {
		s.t.Fatal(err)
	}
	return tok
}

func (s *stack) create(tenant string, payload map[string]any) reply {
	return s.do("POST", "/api/api-keys", s.owner(tenant), payload)
}

func TestAKeyIsMintedOnceHashedListedByPrefixAndAuthenticatesItself(t *testing.T) {
	s := newStack(t)
	r := s.create(s.a, map[string]any{"name": "ci", "permissions": []string{"api_key.view", "example.view", "example.view"}, "expiresInDays": 30})
	if r.code != 201 || !strings.HasPrefix(r.str("secret"), "sk_") || r.str("warning") == "" || r.str("prefix") != r.str("secret")[:12] || r.body["expiresAt"] == nil {
		t.Fatalf("create: %d %v", r.code, r.body)
	}
	if perms, _ := r.body["permissions"].([]any); len(perms) != 2 {
		t.Fatalf("permissions are deduplicated and sorted: %v", perms)
	}
	secret := r.str("secret")
	// The table holds the hash, not the secret.
	var stored string
	if err := s.db.Admin.QueryRow(s.ctx, `SELECT key_hash FROM api_key WHERE id = $1`, r.str("id")).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == secret || stored != auth.HashKey(secret) || len(stored) != 64 {
		t.Fatalf("stored %q for secret %q", stored, secret)
	}
	// The key lists keys (it holds api_key.view) and sees no secret.
	l := s.do("GET", "/api/api-keys", secret, nil)
	items, _ := l.body["items"].([]any)
	if l.code != 200 || len(items) != 1 {
		t.Fatalf("list as the key: %d %v", l.code, l.body)
	}
	raw, _ := json.Marshal(l.body)
	if strings.Contains(string(raw), secret) || !strings.Contains(string(raw), `"prefix":"`+secret[:12]+`"`) || !strings.Contains(string(raw), `"lastUsedAt":"`) {
		t.Fatalf("list leaks or lacks: %s", raw)
	}
	// The key may not do what it was not granted.
	if c := s.do("POST", "/api/api-keys", secret, map[string]any{"name": "x", "permissions": []string{"example.view"}}); c.code != 403 {
		t.Fatalf("a key minting keys without api_key.manage: %d", c.code)
	}
	// B's owner sees nothing of A's.
	if l := s.do("GET", "/api/api-keys", s.owner(s.b), nil); l.code != 200 || len(l.body["items"].([]any)) != 0 {
		t.Fatalf("B list: %d %v", l.code, l.body)
	}
	if e := entries(s, "api_key.created"); len(e) != 1 || e[0].TenantID != s.a || e[0].After["prefix"] != secret[:12] || e[0].After["hash"] != nil {
		t.Fatalf("audit: %+v", e)
	}
}

func TestRefusedKeysAndRefusedRequests(t *testing.T) {
	s := newStack(t)
	for _, bad := range []map[string]any{
		{"name": "", "permissions": []string{"example.view"}},
		{"name": "x", "permissions": []string{}},
		{"name": "x", "permissions": []string{"nothing.declares.this"}},
	} {
		if r := s.create(s.a, bad); r.code != 400 {
			t.Fatalf("%v: %d %v", bad, r.code, r.body)
		}
	}
	// A hundred characters, as the spec says - not a hundred bytes.
	if r := s.create(s.a, map[string]any{"name": strings.Repeat("ą", 100), "permissions": []string{"example.view"}}); r.code != 201 {
		t.Fatalf("100 two-byte letters: %d %v", r.code, r.body)
	}
	if r := s.create(s.a, map[string]any{"name": strings.Repeat("ą", 101), "permissions": []string{"example.view"}}); r.code != 400 {
		t.Fatalf("101 characters: %d %v", r.code, r.body)
	}
	if r := s.do("POST", "/api/api-keys", "", map[string]any{"name": "x", "permissions": []string{"example.view"}}); r.code != 401 {
		t.Fatalf("anonymous: %d", r.code)
	}
	admin, _ := s.signer.Sign(auth.Claims{Subject: "u", TenantID: s.a, Audience: auth.RealmTenant, Permissions: []string{"api_key.view"}}, time.Hour)
	if r := s.do("POST", "/api/api-keys", admin, map[string]any{"name": "x", "permissions": []string{"example.view"}}); r.code != 403 {
		t.Fatalf("without api_key.manage: %d", r.code)
	}
	// A credential cannot outrank the one that made it.
	narrow, _ := s.signer.Sign(auth.Claims{Subject: "u", TenantID: s.a, Audience: auth.RealmTenant, Permissions: []string{"api_key.manage", "example.view"}}, time.Hour)
	if r := s.do("POST", "/api/api-keys", narrow, map[string]any{"name": "x", "permissions": []string{"example.view", "example.manage"}}); r.code != 403 || r.str("error", "code") != "forbidden" || r.str("error", "details", "permission") != "example.manage" {
		t.Fatalf("a permission the caller lacks: %d %v", r.code, r.body)
	}
	if r := s.do("POST", "/api/api-keys", narrow, map[string]any{"name": "x", "permissions": []string{"example.view"}}); r.code != 201 {
		t.Fatalf("a permission the caller holds: %d %v", r.code, r.body)
	}
	// A support session may look, not mint or revoke.
	viewing, _ := s.signer.Sign(auth.Claims{Subject: "u", TenantID: s.a, Audience: auth.RealmTenant, Permissions: s.svc.Permissions,
		Impersonated: true, Actor: &auth.ActorInfo{Subject: "op", Realm: auth.RealmManager}}, time.Hour)
	if r := s.do("POST", "/api/api-keys", viewing, map[string]any{"name": "x", "permissions": []string{"example.view"}}); r.code != 403 {
		t.Fatalf("minted under impersonation: %d %v", r.code, r.body)
	}
	if r := s.do("DELETE", "/api/api-keys/00000000-0000-0000-0000-000000000000", viewing, nil); r.code != 403 {
		t.Fatalf("revoked under impersonation: %d %v", r.code, r.body)
	}
}

func TestRevokedExpiredAndSuspendedKeysStopAuthenticating(t *testing.T) {
	s := newStack(t)
	live := s.create(s.a, map[string]any{"name": "live", "permissions": []string{"api_key.view"}})
	revoked := s.create(s.a, map[string]any{"name": "gone", "permissions": []string{"api_key.view"}})
	expired := s.create(s.a, map[string]any{"name": "old", "permissions": []string{"api_key.view"}, "expiresInDays": 1})
	if r := s.do("DELETE", "/api/api-keys/"+revoked.str("id"), s.owner(s.a), nil); r.code != 200 || r.str("status") != "revoked" {
		t.Fatalf("revoke: %d %v", r.code, r.body)
	}
	if r := s.do("DELETE", "/api/api-keys/"+revoked.str("id"), s.owner(s.a), nil); r.code != 200 {
		t.Fatalf("revoking twice: %d", r.code)
	}
	if r := s.do("DELETE", "/api/api-keys/"+live.str("id"), s.owner(s.b), nil); r.code != 404 {
		t.Fatalf("B revoking A's key: %d", r.code)
	}
	if _, err := s.db.Admin.Exec(s.ctx, `UPDATE api_key SET expires_at = now() - interval '1 minute' WHERE id = $1`, expired.str("id")); err != nil {
		t.Fatal(err)
	}
	if r := s.do("GET", "/api/api-keys", live.str("secret"), nil); r.code != 200 {
		t.Fatalf("live key: %d", r.code)
	}
	l := s.do("GET", "/api/api-keys", s.owner(s.a), nil)
	if items, _ := l.body["items"].([]any); len(items) != 3 {
		t.Fatalf("a revoked key stays in the list: %v", l.body)
	}
	var msgs []string
	for name, secret := range map[string]string{"revoked": revoked.str("secret"), "expired": expired.str("secret"), "unknown": "sk_" + strings.Repeat("x", 43)} {
		r := s.do("GET", "/api/api-keys", secret, nil)
		if r.code != 401 {
			t.Fatalf("%s key: %d %v", name, r.code, r.body)
		}
		msgs = append(msgs, r.str("error", "message"))
	}
	if msgs[0] != msgs[1] || msgs[1] != msgs[2] {
		t.Fatalf("refusals differ: %v", msgs)
	}
	if err := (tenantc.Provisioner{Bus: s.svc.Bus}).Suspend(s.ctx, s.a); err != nil {
		t.Fatal(err)
	}
	if r := s.do("GET", "/api/api-keys", live.str("secret"), nil); r.code != 401 {
		t.Fatalf("a suspended tenant's key still works: %d", r.code)
	}
	// One real revocation, one idempotent repeat, one refused for B: all three in the trail.
	if e := entries(s, "api_key.revoked"); len(e) != 3 || e[0].Before["revoked"] != false || e[0].After["revoked"] != true || e[2].Succeeded {
		t.Fatalf("audit: %+v", e)
	}
}

func TestTheConsoleMintsForATenantBySlug(t *testing.T) {
	s := newStack(t)
	secret, id, prefix, err := api_key.CreateKey(s.ctx, s.svc, api_key.CreateKeyInput{Tenant: "b", Name: "cli", Permissions: []string{"api_key.view"}, Days: 7})
	if err != nil || !strings.HasPrefix(secret, prefix) || id == "" {
		t.Fatalf("console: %v %q %q", err, secret, prefix)
	}
	l := s.do("GET", "/api/api-keys", secret, nil)
	if items, _ := l.body["items"].([]any); l.code != 200 || len(items) != 1 || items[0].(map[string]any)["id"] != id {
		t.Fatalf("the console key lists B's keys: %d %v", l.code, l.body)
	}
	if _, _, _, err := api_key.CreateKey(s.ctx, s.svc, api_key.CreateKeyInput{Tenant: "nope", Name: "x", Permissions: []string{"api_key.view"}}); err == nil {
		t.Fatal("unknown tenant accepted")
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
