package manager_test

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

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"

	kaudit "github.com/open-enu/app/internal/module/audit"
	auditc "github.com/open-enu/app/internal/module/audit/contract"
	"github.com/open-enu/app/internal/module/identity"
	"github.com/open-enu/app/internal/module/manager"
	"github.com/open-enu/app/internal/module/tenant"
)

// The realm from outside: an operator logs in, sees every workspace, hands
// a session to a member, and is refused everywhere a tenant token works.

const (
	opEmail    = "operator@example.test"
	opPassword = "an-operator-password-of-sixteen"
	userEmail  = "owner@example.test"
	password   = "correct-horse-battery-staple"
)

type stack struct {
	t    *testing.T
	ctx  context.Context
	svc  *platform.Services
	h    http.Handler
	a, b string // tenant ids
	user string // the owner's user id
}

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil,
		migrate.Source{Name: "tenant", FS: tenant.Module.Migrations},
		migrate.Source{Name: "identity", FS: identity.Module.Migrations},
		migrate.Source{Name: "manager", FS: manager.Module.Migrations}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.Actor = auth.Actor{}
	svc.Permissions = append(append(append([]string{}, tenant.Module.Permissions...), identity.Module.Permissions...), kaudit.Module.Permissions...)
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 1, PollInterval: time.Second}, true); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	for _, m := range []func(*http.ServeMux, *platform.Services){tenant.Module.Mount, identity.Module.Mount, kaudit.Module.Mount, manager.Module.Mount} {
		m(mux, svc)
	}
	s := &stack{t: t, ctx: ctx, svc: svc, h: httpx.Chain(mux, httpx.RequestID, httpx.Recover, auth.Middleware(svc.Signer, nil))}
	primary, err := identity.CreateUser(ctx, svc, identity.CreateUserInput{Email: userEmail, Password: password, DisplayName: "Owner", TenantSlug: "acme", TenantName: "Acme", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := identity.CreateUser(ctx, svc, identity.CreateUserInput{Email: "other@example.test", Password: password, TenantSlug: "beta", TenantName: "Beta", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	s.a, s.b, s.user = primary.TenantID, second.TenantID, primary.UserID
	if _, created, err := manager.CreateOperator(ctx, svc, manager.CreateOperatorInput{Email: opEmail, Password: opPassword, DisplayName: "Op", Super: true}); err != nil || !created {
		t.Fatalf("operator: %v %v", created, err)
	}
	return s
}

type reply struct {
	code    int
	raw     []byte
	body    map[string]any
	cookies []*http.Cookie
}

func (r reply) str(k string) string { v, _ := r.body[k].(string); return v }

func (s *stack) do(method, path, authz string, payload any) reply {
	s.t.Helper()
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "10.0.0.9:1234"
	req.Header.Set("Content-Type", "application/json")
	if authz != "" {
		req.Header.Set("Authorization", "Bearer "+authz)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	r := reply{code: rec.Code, raw: rec.Body.Bytes(), cookies: rec.Result().Cookies()}
	_ = json.Unmarshal(r.raw, &r.body)
	return r
}

// trail is every entry, oldest first, through the contract the manager
// route itself reads.
func (s *stack) trail() []auditc.Entry {
	s.t.Helper()
	entries, err := auditc.NewReader(s.svc).Recent(s.ctx, nil, 200)
	if err != nil {
		s.t.Fatal(err)
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries
}

func (s *stack) operator() string {
	s.t.Helper()
	r := s.do("POST", "/api/manager/login", "", map[string]any{"email": opEmail, "password": opPassword})
	if r.code != 200 || r.str("token") == "" {
		s.t.Fatalf("operator login: %d %s", r.code, r.raw)
	}
	return r.str("token")
}

func (s *stack) owner() string {
	s.t.Helper()
	r := s.do("POST", "/api/auth/login", "", map[string]any{"email": userEmail, "password": password})
	if r.code != 200 {
		s.t.Fatalf("owner login: %d %s", r.code, r.raw)
	}
	return r.str("token")
}

func TestOperatorLoginIsItsOwnRealmWithNoRefreshAndOneRefusal(t *testing.T) {
	s := newStack(t)
	r := s.do("POST", "/api/manager/login", "", map[string]any{"email": strings.ToUpper(opEmail), "password": opPassword})
	if r.code != 200 || r.body["expiresIn"] != float64(900) || len(r.cookies) != 0 {
		t.Fatalf("login: %d %s cookies %d", r.code, r.raw, len(r.cookies))
	}
	m := r.body["manager"].(map[string]any)
	if m["email"] != opEmail || m["displayName"] != "Op" || m["status"] != "active" || m["lastLoginAt"] == nil {
		t.Fatalf("manager: %v", m)
	}
	claims, err := s.svc.Signer.Verify(r.str("token"))
	if err != nil || claims.Audience != auth.RealmManager || claims.TenantID != "" {
		t.Fatalf("claims: %+v %v", claims, err)
	}
	// Second operator with the same address: found, not duplicated.
	if _, created, err := manager.CreateOperator(s.ctx, s.svc, manager.CreateOperatorInput{Email: opEmail, Password: opPassword}); err != nil || created {
		t.Fatalf("idempotent create: %v %v", created, err)
	}
	if _, _, err := manager.CreateOperator(s.ctx, s.svc, manager.CreateOperatorInput{Email: "x@example.test", Password: "short"}); err == nil {
		t.Fatal("a short operator password was accepted")
	}
	var bodies []string
	for _, in := range []map[string]any{{"email": "nobody@example.test", "password": opPassword}, {"email": opEmail, "password": "wrong-" + opPassword}} {
		x := s.do("POST", "/api/manager/login", "", in)
		if x.code != 401 {
			t.Fatalf("%v: %d", in, x.code)
		}
		delete(x.body["error"].(map[string]any), "requestId")
		b, _ := json.Marshal(x.body)
		bodies = append(bodies, string(b))
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("refusals differ: %v", bodies)
	}
	// The strictest limit: five attempts, then 429.
	var limited bool
	for i := 0; i < 6; i++ {
		if x := s.do("POST", "/api/manager/login", "", map[string]any{"email": "a@example.test", "password": opPassword}); x.code == 429 {
			limited = true
		}
	}
	if !limited {
		t.Fatal("no rate limit on operator login")
	}
	// The limiter is per route: the owner's login is not caught in it.
	s.owner()
}

func TestTheOperatorSeesEveryWorkspaceAndATenantTokenSeesNoneOfThis(t *testing.T) {
	s := newStack(t)
	op, owner := s.operator(), s.owner()
	l := s.do("GET", "/api/manager/tenants?size=10", op, nil)
	items, _ := l.body["items"].([]any)
	if l.code != 200 || len(items) != 2 || l.body["meta"].(map[string]any)["total"] != float64(2) {
		t.Fatalf("list: %d %s", l.code, l.raw)
	}
	for _, it := range items {
		if it.(map[string]any)["members"] != float64(1) {
			t.Fatalf("members: %v", it)
		}
	}
	d := s.do("GET", "/api/manager/tenants/"+s.a, op, nil)
	members, _ := d.body["members"].([]any)
	if d.code != 200 || d.str("slug") != "acme" || len(members) != 1 || members[0].(map[string]any)["id"] != s.user || members[0].(map[string]any)["email"] != userEmail || members[0].(map[string]any)["role"] != "owner" {
		t.Fatalf("detail: %d %s", d.code, d.raw)
	}
	if x := s.do("GET", "/api/manager/tenants/00000000-0000-0000-0000-000000000000", op, nil); x.code != 404 {
		t.Fatalf("unknown tenant: %d", x.code)
	}
	for _, path := range []string{"/api/manager/tenants", "/api/manager/tenants/" + s.a, "/api/manager/audit", "/api/manager/workers"} {
		if x := s.do("GET", path, owner, nil); x.code != 401 {
			t.Fatalf("owner on %s: %d", path, x.code)
		}
		if x := s.do("GET", path, "", nil); x.code != 401 {
			t.Fatalf("anonymous on %s: %d", path, x.code)
		}
	}
	if x := s.do("POST", "/api/manager/tenants/"+s.a+"/users/"+s.user+"/impersonate", owner, nil); x.code != 401 {
		t.Fatalf("an owner minting an impersonation: %d", x.code)
	}
	// And the other way: the operator token is refused on the tenant API.
	if x := s.do("GET", "/api/audit", op, nil); x.code != 401 {
		t.Fatalf("operator on the tenant API: %d", x.code)
	}
	if x := s.do("GET", "/api/profile", op, nil); x.code == 200 {
		t.Fatalf("operator reading a profile: %d", x.code)
	}
	// Workers: River's tables exist, nothing is waiting.
	w := s.do("GET", "/api/manager/workers", op, nil)
	if w.code != 200 || w.body["failed"] != float64(0) || w.body["queues"] == nil {
		t.Fatalf("workers: %d %s", w.code, w.raw)
	}
}

func TestSuspendingAWorkspaceIsAuditedTwiceAndLocksItsMembersOut(t *testing.T) {
	s := newStack(t)
	op := s.operator()
	// A member of Beta with a live session: its refresh must end with the workspace.
	before := s.do("POST", "/api/auth/login", "", map[string]any{"email": "other@example.test", "password": password})
	var cookie *http.Cookie
	for _, c := range before.cookies {
		if c.Name == identity.RefreshCookie {
			cookie = c
		}
	}
	if before.code != 200 || cookie == nil {
		t.Fatalf("login before: %d %s", before.code, before.raw)
	}
	r := s.do("POST", "/api/manager/tenants/"+s.b+"/suspend", op, nil)
	if r.code != 200 || r.str("status") != "suspended" {
		t.Fatalf("suspend: %d %s", r.code, r.raw)
	}
	if d := s.do("GET", "/api/manager/tenants/"+s.b, op, nil); d.str("status") != "suspended" {
		t.Fatalf("status: %s", d.raw)
	}
	if l := s.do("POST", "/api/auth/login", "", map[string]any{"email": "other@example.test", "password": password}); l.code != 401 {
		t.Fatalf("a member of a suspended workspace logged in: %d", l.code)
	}
	req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
	req.RemoteAddr = "10.0.0.9:1234"
	req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	if rec.Code != 401 || strings.Contains(rec.Body.String(), `"token"`) {
		t.Fatalf("a refresh into a suspended workspace: %d %s", rec.Code, rec.Body.String())
	}
	actions := map[string]bool{}
	for _, e := range s.trail() {
		if e.TenantID != nil && *e.TenantID == s.b && e.Succeeded {
			actions[e.Action] = true
		}
	}
	if !actions["manager.tenant.suspended"] || !actions["tenant.suspended"] || !actions["identity.sessions.ended"] {
		t.Fatalf("trail: %v", actions)
	}
}

func TestImpersonationMintsAMarkedTenantTokenWithNoCookieAndIsAuditedFirst(t *testing.T) {
	s := newStack(t)
	op := s.operator()
	r := s.do("POST", "/api/manager/tenants/"+s.a+"/users/"+s.user+"/impersonate", op, nil)
	if r.code != 200 || r.body["impersonating"] != true || len(r.cookies) != 0 {
		t.Fatalf("impersonate: %d %s cookies %d", r.code, r.raw, len(r.cookies))
	}
	if r.body["operator"].(map[string]any)["displayName"] != "Op" || r.body["tenantId"] != s.a || r.body["user"].(map[string]any)["email"] != userEmail || r.body["role"] != "owner" {
		t.Fatalf("session: %s", r.raw)
	}
	claims, err := s.svc.Signer.Verify(r.str("token"))
	if err != nil || claims.Audience != auth.RealmTenant || !claims.Impersonated || claims.Actor == nil || claims.Actor.Realm != auth.RealmManager || claims.Subject != s.user {
		t.Fatalf("claims: %+v %v", claims, err)
	}
	// The token works on the tenant API as the person, and the trail names both.
	if x := s.do("GET", "/api/audit", r.str("token"), nil); x.code != 200 {
		t.Fatalf("impersonated read: %d %s", x.code, x.raw)
	}
	var started *auditc.Entry
	for _, e := range s.trail() {
		if e.Action == "manager.impersonation.started" {
			started = &e
		}
	}
	if started == nil || *started.TenantID != s.a || *started.SubjectID != s.user || *started.ActorID != claims.Actor.Subject {
		t.Fatalf("audit: %+v", started)
	}
	// Not a member of that workspace: refused, but the attempt is on record.
	if x := s.do("POST", "/api/manager/tenants/"+s.b+"/users/"+s.user+"/impersonate", op, nil); x.code != 400 {
		t.Fatalf("impersonating a non-member: %d %s", x.code, x.raw)
	}
	var attempts int
	for _, e := range s.trail() {
		if e.Action == "manager.impersonation.started" && e.TenantID != nil && *e.TenantID == s.b {
			attempts++
		}
	}
	if attempts != 1 {
		t.Fatalf("the refused attempt is not in the trail: %d", attempts)
	}
	if x := s.do("POST", "/api/manager/tenants/"+s.a+"/users/00000000-0000-0000-0000-000000000000/impersonate", op, nil); x.code != 404 {
		t.Fatalf("unknown user: %d", x.code)
	}
	// Cross-tenant audit, and filtered to one.
	all := s.do("GET", "/api/manager/audit", op, nil)
	only := s.do("GET", "/api/manager/audit?tenantId="+s.b+"&limit=5", op, nil)
	if all.code != 200 || len(all.body["items"].([]any)) < 4 || only.code != 200 {
		t.Fatalf("audit: %d %d", all.code, only.code)
	}
	for _, it := range only.body["items"].([]any) {
		if it.(map[string]any)["tenantId"] != s.b {
			t.Fatalf("filter leaked: %v", it)
		}
	}
}

func TestOnlyASuperOperatorMayImpersonateAndAPlainOnesAttemptIsOnRecord(t *testing.T) {
	s := newStack(t)
	if _, _, err := manager.CreateOperator(s.ctx, s.svc, manager.CreateOperatorInput{Email: "plain@example.test", Password: opPassword, DisplayName: "Plain"}); err != nil {
		t.Fatal(err)
	}
	l := s.do("POST", "/api/manager/login", "", map[string]any{"email": "plain@example.test", "password": opPassword})
	if l.code != 200 {
		t.Fatalf("plain login: %d %s", l.code, l.raw)
	}
	plain := l.str("token")
	// A plain operator still sees the workspace...
	if d := s.do("GET", "/api/manager/tenants/"+s.a, plain, nil); d.code != 200 {
		t.Fatalf("plain operator reads: %d", d.code)
	}
	// ...and may not become one of its members. (This stack loads no
	// catalogue, so the message is its key; the security suite reads the text.)
	r := s.do("POST", "/api/manager/tenants/"+s.a+"/users/"+s.user+"/impersonate", plain, nil)
	if r.code != 403 || r.str("token") != "" || !strings.Contains(string(r.raw), `"code":"forbidden"`) || !strings.Contains(string(r.raw), "super_required") {
		t.Fatalf("plain operator impersonated: %d %s", r.code, r.raw)
	}
	var refused bool
	for _, e := range s.trail() {
		if e.Action == "manager.impersonation.started" && !e.Succeeded && e.SubjectID != nil && *e.SubjectID == s.user {
			refused = true
		}
	}
	if !refused {
		t.Fatal("the refused attempt is not on the trail")
	}
	// The super operator may.
	if ok := s.do("POST", "/api/manager/tenants/"+s.a+"/users/"+s.user+"/impersonate", s.operator(), nil); ok.code != 200 || ok.body["impersonating"] != true || ok.str("token") == "" {
		t.Fatalf("super operator: %d %s", ok.code, ok.raw)
	}
}
