package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/mail"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/identity"
	"github.com/open-enu/app/internal/module/tenant"
	tenantc "github.com/open-enu/app/internal/module/tenant/contract"
)

// Every route through real HTTP against real Postgres, with the Tenant
// module mounted beside it: a session names its workspaces through
// Tenant's contract, and a stand-in would test the stand-in.

const (
	email    = "owner@example.test"
	password = "correct-horse-battery-staple"
)

type stack struct {
	t    *testing.T
	ctx  context.Context
	db   *testdb.DB
	svc  *platform.Services
	h    http.Handler
	mail *mail.Recorder
	keys auth.MemoryKeys
}

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil,
		migrate.Source{Name: "tenant", FS: tenant.Module.Migrations},
		migrate.Source{Name: "identity", FS: identity.Module.Migrations}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.Actor = auth.Actor{}
	svc.Permissions = append(append([]string{}, tenant.Module.Permissions...), identity.Module.Permissions...)
	svc.Permissions = append(svc.Permissions, "example.view", "example.manage", "api_key.manage")
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 2, PollInterval: 100 * time.Millisecond}, false); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	tenant.Module.Mount(mux, svc)
	identity.Module.Mount(mux, svc)
	keys := auth.MemoryKeys{}
	// The two locale preferences in the order the process asks them.
	reg, err := module.NewRegistry(tenant.Module, identity.Module)
	if err != nil {
		t.Fatal(err)
	}
	return &stack{t: t, ctx: ctx, db: db, svc: svc, mail: svc.Mail.(*mail.Recorder), keys: keys,
		h: httpx.Chain(mux, httpx.RequestID, httpx.Recover, auth.Middleware(svc.Signer, keys), httpx.Locale(svc.I18n, reg.LocalePreferences(svc)...))}
}

type reply struct {
	code    int
	body    map[string]any
	cookies []*http.Cookie
	header  http.Header
}

func (r reply) str(path ...string) string { v, _ := r.at(path...).(string); return v }
func (r reply) at(path ...string) any {
	var cur any = r.body
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

// refresh is the refresh cookie the response set, or nil.
func (r reply) refresh() *http.Cookie {
	for _, c := range r.cookies {
		if c.Name == identity.RefreshCookie {
			return c
		}
	}
	return nil
}

func (s *stack) do(method, path, authz string, payload any, cookie *http.Cookie) reply {
	s.t.Helper()
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "10.0.0.7:1234"
	req.Header.Set("Content-Type", "application/json")
	if authz != "" {
		req.Header.Set("Authorization", "Bearer "+authz)
	}
	if cookie != nil {
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	r := reply{code: rec.Code, cookies: rec.Result().Cookies(), header: rec.Header()}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &r.body); err != nil {
			s.t.Fatalf("%s %s: not JSON: %s", method, path, rec.Body.String())
		}
	}
	return r
}

func (s *stack) post(path string, payload any) reply { return s.do("POST", path, "", payload, nil) }

var tokenInLink = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

// tenantOf is the first workspace address belongs to. The registration
// answer does not name it - it is the same for an address that already has
// an account (ADR-0023) - so a test reads the table.
func (s *stack) tenantOf(address string) string {
	s.t.Helper()
	var id string
	if err := s.db.Admin.QueryRow(s.ctx, `SELECT m.tenant_id FROM identity_membership m JOIN identity_user u ON u.id = m.user_id
		WHERE u.email_hash = $1 ORDER BY m.created_at LIMIT 1`, crypto.HashForLookup(address)).Scan(&id); err != nil {
		s.t.Fatalf("no workspace for %s: %v", address, err)
	}
	return id
}

// firstMail is the first message sent to address - the verification link,
// when a later attempt has mailed the owner since.
func (s *stack) firstMail(address string) mail.Message {
	s.t.Helper()
	for _, m := range s.mail.Sent() {
		if m.To == address {
			return m
		}
	}
	s.t.Fatalf("nothing was sent to %s", address)
	return mail.Message{}
}

// register runs register + verify for email and returns the tenant id.
func (s *stack) register(name string) string {
	s.t.Helper()
	r := s.post("/api/auth/register", map[string]any{"email": email, "password": password, "tenantName": name, "displayName": "Owner"})
	if r.code != 202 || r.str("status") != "unverified" || len(r.body) != 1 {
		s.t.Fatalf("register: %d %v", r.code, r.body)
	}
	m, ok := s.mail.Last(email)
	if !ok {
		s.t.Fatal("no verification mail")
	}
	tok := tokenInLink.FindStringSubmatch(m.Text)
	if tok == nil || !strings.Contains(m.Text, "http://app.test/verify?token=") {
		s.t.Fatalf("mail without a link: %q", m.Text)
	}
	if v := s.post("/api/auth/verify", map[string]any{"token": tok[1]}); v.code != 200 || v.str("status") != "verified" || v.str("userId") == "" {
		s.t.Fatalf("verify: %d %v", v.code, v.body)
	}
	return s.tenantOf(email)
}

func (s *stack) login() reply {
	s.t.Helper()
	r := s.post("/api/auth/login", map[string]any{"email": email, "password": password})
	if r.code != 200 || r.refresh() == nil {
		s.t.Fatalf("login: %d %v", r.code, r.body)
	}
	return r
}

func (s *stack) entries(action string) (out []audit.Entry) {
	for _, e := range s.svc.Audit.(*audit.Recorder).Entries() {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

func (s *stack) work(n int) {
	s.t.Helper()
	done, cancel := s.svc.Outbox.Completed()
	defer cancel()
	if err := s.svc.Outbox.Start(s.ctx); err != nil {
		s.t.Fatal(err)
	}
	defer s.svc.Outbox.Stop(s.ctx)
	for i := 0; i < n; i++ {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			s.t.Fatalf("job %d of %d never finished", i+1, n)
		}
	}
}

func TestRegisterVerifyAndLoginIssueASessionAndACookie(t *testing.T) {
	s := newStack(t)
	// Before verifying: the tenant is pending and the login is refused with
	// the same 401 as a wrong password.
	r := s.post("/api/auth/register", map[string]any{"email": email, "password": password, "tenantName": "Acme"})
	if r.code != 202 {
		t.Fatalf("register: %d %v", r.code, r.body)
	}
	if l := s.post("/api/auth/login", map[string]any{"email": email, "password": password}); l.code != 401 || l.str("error", "code") != "unauthenticated" {
		t.Fatalf("unverified login: %d %v", l.code, l.body)
	}
	tenantID := s.tenantOf(email)
	// The same address again: the same answer, nothing created, and the
	// owner told by mail - "that address has an account" would tell anyone
	// who asked (ADR-0023).
	if again := s.post("/api/auth/register", map[string]any{"email": email, "password": password, "tenantName": "Acme two"}); again.code != 202 || !reflect.DeepEqual(again.body, r.body) {
		t.Fatalf("second register with the same address: %d %v, the first said %v", again.code, again.body, r.body)
	}
	var workspaces int
	if err := s.db.Admin.QueryRow(s.ctx, "SELECT count(*) FROM tenant").Scan(&workspaces); err != nil || workspaces != 1 {
		t.Fatalf("a second registration created a workspace: %d %v", workspaces, err)
	}
	// Not verified yet: they most likely lost the first link, so they get a
	// new one rather than "sign in", which would not let them.
	resent, _ := s.mail.Last(email)
	if first := s.firstMail(email); resent.Text == first.Text || !tokenInLink.MatchString(resent.Text) {
		t.Fatalf("an unverified owner is not sent a new link: %+v", resent)
	}
	if e := s.entries("identity.user.registered"); len(e) != 2 || e[1].After["accountExisted"] != true {
		t.Fatalf("the attempt is on the trail as what it was: %+v", e)
	}
	// Lengths are the spec's, in characters: a 200-letter Polish workspace
	// name and a twelve-letter password pass, through Identity's check and
	// Tenant's; one letter more of the name does not, nor eleven of the password.
	polish := map[string]any{"email": "polish@example.test", "password": strings.Repeat("ą", 12), "tenantName": strings.Repeat("ż", 200), "displayName": strings.Repeat("ł", 120)}
	if r := s.post("/api/auth/register", polish); r.code != 202 {
		t.Fatalf("200-character name, 12-character password: %d %v", r.code, r.body)
	}
	for field, value := range map[string]string{"tenantName": strings.Repeat("ż", 201), "password": strings.Repeat("ą", 11)} {
		bad := map[string]any{"email": "polish-" + field + "@example.test", "password": strings.Repeat("ą", 12), "tenantName": "Polska"}
		bad[field] = value
		if r := s.post("/api/auth/register", bad); r.code != 400 {
			t.Fatalf("%s one past the limit: %d %v", field, r.code, r.body)
		}
	}
	if short := s.post("/api/auth/register", map[string]any{"email": "x@example.test", "password": "short", "tenantName": "X"}); short.code != 400 {
		t.Fatalf("short password: %d", short.code)
	}
	info, found, _ := tenantc.NewReader(s.svc).Describe(s.ctx, tenantID)
	if !found || info.Active() {
		t.Fatalf("tenant before verify: %+v", info)
	}
	m := s.firstMail(email)
	tok := tokenInLink.FindStringSubmatch(m.Text)[1]
	if v := s.post("/api/auth/verify", map[string]any{"token": tok}); v.code != 200 {
		t.Fatalf("verify: %d %v", v.code, v.body)
	}
	if v := s.post("/api/auth/verify", map[string]any{"token": tok}); v.code != 400 {
		t.Fatalf("a verification token is single use: %d", v.code)
	}
	// Verified: the same answer again, and the owner is told they have an
	// account, with the way in and the way back in - and no token.
	if again := s.post("/api/auth/register", map[string]any{"email": email, "password": password, "tenantName": "Acme three"}); again.code != 202 || !reflect.DeepEqual(again.body, r.body) {
		t.Fatalf("register after verifying: %d %v", again.code, again.body)
	}
	if m, _ := s.mail.Last(email); !strings.Contains(m.Text, "http://app.test/login") || !strings.Contains(m.Text, "http://app.test/forgot-password") || tokenInLink.MatchString(m.Text) {
		t.Fatalf("a verified owner is not told: %+v", m)
	}
	if info, _, _ = tenantc.NewReader(s.svc).Describe(s.ctx, tenantID); !info.Active() {
		t.Fatalf("tenant after verify: %+v", info)
	}

	l := s.login()
	if l.str("user", "email") != email || l.str("tenantId") != tenantID || l.str("role") != "owner" || l.at("expiresIn") != float64(900) {
		t.Fatalf("session: %v", l.body)
	}
	ms, _ := l.body["memberships"].([]any)
	if len(ms) != 1 || ms[0].(map[string]any)["tenantName"] != "Acme" || ms[0].(map[string]any)["tenantSlug"] != info.Slug {
		t.Fatalf("memberships named through Tenant: %v", ms)
	}
	perms, _ := l.body["permissions"].([]any)
	if len(perms) != len(s.svc.Permissions) {
		t.Fatalf("an owner holds every declared permission, got %v", perms)
	}
	c := l.refresh()
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/api/auth" || c.MaxAge < 29*24*3600 || c.Secure {
		t.Fatalf("cookie attributes (plain-http platform): %+v", c)
	}
	raw, _ := json.Marshal(l.body)
	if strings.Contains(string(raw), c.Value) {
		t.Fatal("the refresh token is in the body")
	}
	// The access token opens the profile and carries the tenant.
	claims, err := s.svc.Signer.Verify(l.str("token"))
	if err != nil || claims.TenantID != tenantID || claims.Audience != auth.RealmTenant || len(claims.Permissions) != len(perms) {
		t.Fatalf("claims: %+v %v", claims, err)
	}
	if p := s.do("GET", "/api/profile", l.str("token"), nil, nil); p.code != 200 || p.str("email") != email || p.str("displayName") != "" {
		t.Fatalf("profile: %d %v", p.code, p.body)
	}
	if wrong := s.post("/api/auth/login", map[string]any{"email": email, "password": "wrong-password-entirely"}); wrong.code != 401 || wrong.str("error", "message") != s.post("/api/auth/login", map[string]any{"email": "nobody@example.test", "password": password}).str("error", "message") {
		t.Fatalf("wrong password and unknown address differ: %v", wrong.body)
	}
	if e := s.entries("identity.user.registered"); len(e) < 1 || !e[0].Succeeded || e[0].After["status"] != "unverified" {
		t.Fatalf("audit: %+v", e)
	}
	if e := s.entries("identity.email.verified"); len(e) != 2 || !e[0].Succeeded || e[1].Succeeded {
		t.Fatalf("verify audit (one success, one refused replay): %+v", e)
	}
}

func TestRefreshRotatesAndAReplayRevokesTheFamily(t *testing.T) {
	s := newStack(t)
	s.register("Acme")
	first := s.login().refresh()
	second := s.do("POST", "/api/auth/refresh", "", nil, first)
	if second.code != 200 || second.refresh() == nil || second.refresh().Value == first.Value || second.str("token") == "" {
		t.Fatalf("refresh: %d %v", second.code, second.body)
	}
	// Presenting the consumed token again is a replay: refused, and the
	// token it was rotated into dies with it.
	if replay := s.do("POST", "/api/auth/refresh", "", nil, first); replay.code != 401 {
		t.Fatalf("replay: %d", replay.code)
	}
	if dead := s.do("POST", "/api/auth/refresh", "", nil, second.refresh()); dead.code != 401 {
		t.Fatalf("the family survived a replay: %d", dead.code)
	}
	// No cookie is no credential, not a wrong one: the probe every first
	// visit makes. Nothing to restore, nothing set, nothing cleared.
	if none := s.do("POST", "/api/auth/refresh", "", nil, nil); none.code != 204 || none.body != nil || len(none.cookies) != 0 {
		t.Fatalf("no cookie: %d %v %v", none.code, none.body, none.cookies)
	}
	// A cookie that is sent and does not verify is still a failed credential.
	if junk := s.do("POST", "/api/auth/refresh", "", nil, &http.Cookie{Name: identity.RefreshCookie, Value: "not-a-token"}); junk.code != 401 || junk.str("error", "code") == "" || junk.str("token") != "" {
		t.Fatalf("junk cookie: %d %v", junk.code, junk.body)
	}
}

func TestSwitchTenantAndLogout(t *testing.T) {
	s := newStack(t)
	primary := s.register("Primary")
	// A second workspace, as the console would add it.
	res, err := identity.CreateUser(s.ctx, s.svc, identity.CreateUserInput{Email: email, Password: "ignored-the-account-exists", TenantSlug: "second", TenantName: "Second", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	l := s.login()
	if l.str("tenantId") != primary || len(l.body["memberships"].([]any)) != 2 {
		t.Fatalf("login lands on the owner's workspace: %v", l.body)
	}
	sw := s.do("POST", "/api/auth/switch-tenant", l.str("token"), map[string]any{"tenantId": res.TenantID}, l.refresh())
	if sw.code != 200 || sw.str("tenantId") != res.TenantID || sw.str("role") != "admin" || sw.refresh() == nil {
		t.Fatalf("switch: %d %v", sw.code, sw.body)
	}
	perms, _ := sw.body["permissions"].([]any)
	for _, p := range perms {
		if p == "tenant.delete" || p == "api_key.manage" {
			t.Fatalf("an admin holds an owner-only permission: %v", perms)
		}
	}
	claims, _ := s.svc.Signer.Verify(sw.str("token"))
	if claims.TenantID != res.TenantID {
		t.Fatalf("the new token is still for the old workspace: %+v", claims)
	}
	if bad := s.do("POST", "/api/auth/switch-tenant", "", map[string]any{"tenantId": "0192f000-0000-7000-8000-0000000000ff"}, sw.refresh()); bad.code != 401 {
		t.Fatalf("switch to a workspace without membership: %d %v", bad.code, bad.body)
	}
	// That failed switch consumed its cookie; log in again and log out.
	l = s.login()
	out := s.do("POST", "/api/auth/logout", "", nil, l.refresh())
	if out.code != 200 || out.str("status") != "logged_out" || out.refresh() == nil || out.refresh().MaxAge >= 0 || out.refresh().Value != "" {
		t.Fatalf("logout: %d %v %+v", out.code, out.body, out.refresh())
	}
	if after := s.do("POST", "/api/auth/refresh", "", nil, l.refresh()); after.code != 401 {
		t.Fatalf("refresh after logout: %d", after.code)
	}
	if again := s.do("POST", "/api/auth/logout", "", nil, nil); again.code != 200 {
		t.Fatalf("logout without a cookie still answers: %d", again.code)
	}
}

func TestForgotAndResetPasswordRevokeEverySession(t *testing.T) {
	s := newStack(t)
	s.register("Acme")
	l := s.login()
	before := len(s.mail.Sent())
	unknown := s.post("/api/auth/forgot-password", map[string]any{"email": "nobody@example.test"})
	known := s.post("/api/auth/forgot-password", map[string]any{"email": email})
	if unknown.code != 200 || known.code != 200 || unknown.str("status") != known.str("status") {
		t.Fatalf("forgot answers differ: %v vs %v", unknown.body, known.body)
	}
	if len(s.mail.Sent()) != before+1 {
		t.Fatalf("mails sent: %d, want one (for the known address only)", len(s.mail.Sent())-before)
	}
	m, _ := s.mail.Last(email)
	if !strings.Contains(m.Text, "http://app.test/reset-password?token=") {
		t.Fatalf("reset mail: %q", m.Text)
	}
	tok := tokenInLink.FindStringSubmatch(m.Text)[1]
	if r := s.post("/api/auth/reset-password", map[string]any{"token": tok, "password": "short"}); r.code != 400 {
		t.Fatalf("short new password: %d", r.code)
	}
	r := s.post("/api/auth/reset-password", map[string]any{"token": tok, "password": "a-brand-new-long-password"})
	if r.code != 200 || r.str("status") != "reset" {
		t.Fatalf("reset: %d %v", r.code, r.body)
	}
	if again := s.post("/api/auth/reset-password", map[string]any{"token": tok, "password": "another-long-password-x"}); again.code != 400 {
		t.Fatalf("a reset token is single use: %d", again.code)
	}
	if old := s.post("/api/auth/login", map[string]any{"email": email, "password": password}); old.code != 401 {
		t.Fatalf("old password still works: %d", old.code)
	}
	if fresh := s.post("/api/auth/login", map[string]any{"email": email, "password": "a-brand-new-long-password"}); fresh.code != 200 {
		t.Fatalf("new password: %d %v", fresh.code, fresh.body)
	}
	if dead := s.do("POST", "/api/auth/refresh", "", nil, l.refresh()); dead.code != 401 {
		t.Fatalf("a session from before the reset survived: %d", dead.code)
	}
}

func TestProfileIsForPeopleAndValidatesTheLocale(t *testing.T) {
	s := newStack(t)
	tid := s.register("Acme")
	l := s.login()
	if before := s.do("GET", "/api/profile", l.str("token"), nil, nil); before.header.Get("Content-Language") != "en" {
		t.Fatalf("the workspace's default answers while the person has no choice: %q", before.header.Get("Content-Language"))
	}
	p := s.do("PATCH", "/api/profile", l.str("token"), map[string]any{"displayName": "  Jane  ", "locale": "pl"}, nil)
	if p.code != 200 || p.str("displayName") != "Jane" || p.str("locale") != "pl" || p.at("version") != float64(3) {
		t.Fatalf("patch: %d %v", p.code, p.body)
	}
	// The person's choice outranks the workspace's default, which is always set.
	if after := s.do("GET", "/api/profile", l.str("token"), nil, nil); after.code != 200 || after.str("locale") != "pl" || after.header.Get("Content-Language") != "pl" {
		t.Fatalf("the stored locale is not the response's: %d %q", after.code, after.header.Get("Content-Language"))
	}
	if bad := s.do("PATCH", "/api/profile", l.str("token"), map[string]any{"locale": "de"}, nil); bad.code != 400 {
		t.Fatalf("unsupported locale: %d", bad.code)
	}
	if nobody := s.do("GET", "/api/profile", "", nil, nil); nobody.code != 401 {
		t.Fatalf("anonymous profile: %d", nobody.code)
	}
	key := s.keys.Add("sk_identity-test", auth.Principal{Subject: "key-1", TenantID: tid, Realm: auth.RealmTenant, Permissions: []string{"identity.profile.manage"}})
	if k := s.do("GET", "/api/profile", key, nil, nil); k.code != 403 {
		t.Fatalf("a key has no profile: %d %v", k.code, k.body)
	}
	member, _ := s.svc.Signer.Sign(auth.Claims{Subject: l.str("user", "id"), TenantID: tid, Audience: auth.RealmTenant, Permissions: []string{"example.view"}}, time.Hour)
	if m := s.do("PATCH", "/api/profile", member, map[string]any{"locale": "en"}, nil); m.code != 403 {
		t.Fatalf("without identity.profile.manage: %d", m.code)
	}
	if e := s.entries("identity.profile.updated"); len(e) != 1 || e[0].ActorID != l.str("user", "id") || e[0].TenantID != tid || e[0].Before["version"] != 2 || e[0].After["version"] != 3 {
		t.Fatalf("audit: %+v", e)
	}
}

func TestAuthRoutesAreRateLimitedPerClient(t *testing.T) {
	s := newStack(t)
	var last reply
	for i := 0; i <= 30; i++ {
		last = s.post("/api/auth/login", map[string]any{"email": "nobody@example.test", "password": "whatever-this-is-long"})
	}
	if last.code != 429 || last.str("error", "code") != "rate_limited" || last.header.Get("Retry-After") == "" {
		t.Fatalf("31st login from one client: %d %v", last.code, last.body)
	}
}

func TestADeletedTenantTakesItsMembershipsAndCreateUserIsIdempotent(t *testing.T) {
	s := newStack(t)
	a, err := identity.CreateUser(s.ctx, s.svc, identity.CreateUserInput{Email: email, Password: password, DisplayName: "E2E", TenantSlug: "e2e-primary", TenantName: "E2E Primary", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := identity.CreateUser(s.ctx, s.svc, identity.CreateUserInput{Email: email, Password: password, TenantSlug: "e2e-primary", Role: "owner"})
	if err != nil || b.UserID != a.UserID || b.TenantID != a.TenantID {
		t.Fatalf("second create differs: %+v vs %+v (%v)", a, b, err)
	}
	c, _ := identity.CreateUser(s.ctx, s.svc, identity.CreateUserInput{Email: email, Password: password, TenantSlug: "e2e-second", TenantName: "E2E Second", Role: "owner"})
	l := s.login()
	if l.str("user", "displayName") != "E2E" || len(l.body["memberships"].([]any)) != 2 {
		t.Fatalf("login after console create: %v", l.body)
	}
	if err := (tenantc.Provisioner{Bus: s.svc.Bus}).Delete(s.ctx, c.TenantID); err != nil {
		t.Fatal(err)
	}
	// Two created (the second create found its tenant) + one deleted event,
	// then the membership is gone.
	s.work(3)
	l = s.login()
	if ms := l.body["memberships"].([]any); len(ms) != 1 || ms[0].(map[string]any)["tenantId"] != a.TenantID {
		t.Fatalf("memberships after the tenant went: %v", ms)
	}
	if e := s.entries("identity.memberships.removed"); len(e) != 1 || e[0].TenantID != c.TenantID || e[0].After["removed"] != 1 {
		t.Fatalf("audit: %+v", e)
	}
}
