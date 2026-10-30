// Package security is the suite `make test-security` runs: the process's
// own middleware chain over the real modules against real Postgres, asking
// whether tenant isolation, realm isolation and credential handling hold.
// Nothing here is a unit test; every assertion is made from outside, as an
// attacker would be.
package security_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/flags"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/mail"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/storage"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/api_key"
	"github.com/open-enu/app/internal/module/attachment"
	"github.com/open-enu/app/internal/module/audit"
	"github.com/open-enu/app/internal/module/example"
	"github.com/open-enu/app/internal/module/identity"
	"github.com/open-enu/app/internal/module/manager"
	"github.com/open-enu/app/internal/module/notification"
	"github.com/open-enu/app/internal/module/progress"
	"github.com/open-enu/app/internal/module/search"
	"github.com/open-enu/app/internal/module/settings"
	"github.com/open-enu/app/internal/module/tenant"
	"github.com/open-enu/app/internal/module/webhook"
)

const (
	appOrigin     = "https://app.test"
	managerOrigin = "https://manager.test"
	apiURL        = "https://api.test"
	ownerEmail    = "owner@example.test"
	password      = "correct-horse-battery-staple"
	opEmail       = "operator@example.test"
	opPassword    = "an-operator-password-of-sixteen"
)

type stack struct {
	t    *testing.T
	ctx  context.Context
	db   *testdb.DB
	svc  *platform.Services
	reg  *module.Registry
	h    http.Handler
	mail *mail.Recorder
}

// newStack is cmd/openenu's api, in a test: every module in the registry,
// the same middleware chain in the same order (without the access log),
// HTTPS URLs so cookies come out Secure.
func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	reg, err := module.NewRegistry(tenant.Module, identity.Module, api_key.Module, audit.Module, attachment.Module, progress.Module, settings.Module, manager.Module, example.Module,
		notification.Module, webhook.Module, search.Module)
	if err != nil {
		t.Fatal(err)
	}
	var sources []migrate.Source
	for _, m := range reg.All() {
		sources = append(sources, migrate.Source{Name: m.Manifest.Name, FS: m.Migrations})
	}
	if _, err := migrate.Run(ctx, db.Admin, nil, sources...); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.Actor = auth.Actor{}
	svc.Permissions = reg.Permissions()
	svc.FlagDefinitions = reg.Flags()
	svc.Searchable, svc.NotificationDefinitions = reg.Searchable(), reg.Notifications()
	svc.URLs = platform.URLs{App: appOrigin, Manager: managerOrigin, API: apiURL}
	svc.Storage.(*storage.Memory).Signer.BaseURL = apiURL
	if err := reg.LoadLocales(svc.I18n); err != nil {
		t.Fatal(err)
	}
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 2, PollInterval: 100 * time.Millisecond}, true); err != nil {
		t.Fatal(err)
	}
	// The notifier over Notification's table, as the process builds it.
	notifier, err := reg.Notifier(svc)
	if err != nil {
		t.Fatal(err)
	}
	svc.Notifier = notifier
	// The flag store over Settings' table, as the process builds it.
	store, err := reg.FlagStore(svc)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Flags, err = flags.NewResolver(store, reg.Flags()...); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	reg.Mount(mux, svc)
	storage.Download{Storage: svc.Storage, Signer: svc.Storage.(*storage.Memory).Signer}.Mount(mux)
	keys, err := reg.Keys(svc)
	if err != nil {
		t.Fatal(err)
	}
	h := httpx.Chain(mux, httpx.RequestID, httpx.CORS(appOrigin, managerOrigin), httpx.Recover,
		httpx.Locale(svc.I18n), auth.Middleware(svc.Signer, keys), httpx.Locale(svc.I18n, reg.LocalePreferences(svc)...))
	return &stack{t: t, ctx: ctx, db: db, svc: svc, reg: reg, h: h, mail: svc.Mail.(*mail.Recorder)}
}

type reply struct {
	code    int
	raw     []byte
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
func (r reply) refresh() *http.Cookie {
	for _, c := range r.cookies {
		if c.Name == identity.RefreshCookie {
			return c
		}
	}
	return nil
}

// withoutRequestID is the body with the one field that legitimately differs
// between two identical refusals removed.
func (r reply) withoutRequestID() string {
	if e, ok := r.body["error"].(map[string]any); ok {
		delete(e, "requestId")
	}
	b, _ := json.Marshal(r.body)
	return string(b)
}

type request struct {
	method, path, authz string
	payload             any
	cookie              *http.Cookie
	headers             map[string]string
	rawBody             io.Reader
	contentType         string
}

func (s *stack) send(q request) reply {
	s.t.Helper()
	body := q.rawBody
	if q.payload != nil {
		b, _ := json.Marshal(q.payload)
		body = bytes.NewReader(b)
	}
	req := httptest.NewRequest(q.method, q.path, body)
	req.RemoteAddr = "10.0.0.7:1234"
	req.Header.Set("Content-Type", "application/json")
	if q.contentType != "" {
		req.Header.Set("Content-Type", q.contentType)
	}
	if q.authz != "" {
		req.Header.Set("Authorization", "Bearer "+q.authz)
	}
	if q.cookie != nil {
		req.AddCookie(&http.Cookie{Name: q.cookie.Name, Value: q.cookie.Value})
	}
	for k, v := range q.headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	r := reply{code: rec.Code, raw: rec.Body.Bytes(), cookies: rec.Result().Cookies(), header: rec.Header()}
	_ = json.Unmarshal(r.raw, &r.body)
	return r
}

func (s *stack) do(method, path, authz string, payload any) reply {
	return s.send(request{method: method, path: path, authz: authz, payload: payload})
}

var tokenInLink = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

// mailToken is the token in the last mail sent to the address.
func (s *stack) mailToken(to string) string {
	s.t.Helper()
	m, ok := s.mail.Last(to)
	if !ok {
		s.t.Fatalf("no mail to %s", to)
	}
	tok := tokenInLink.FindStringSubmatch(m.Text)
	if tok == nil {
		s.t.Fatalf("mail without a token: %q", m.Text)
	}
	return tok[1]
}

// register makes a verified owner of a new workspace and returns the tenant id.
func (s *stack) register(email, workspace string) string {
	s.t.Helper()
	r := s.do("POST", "/api/auth/register", "", map[string]any{"email": email, "password": password, "tenantName": workspace})
	if r.code != 202 {
		s.t.Fatalf("register: %d %s", r.code, r.raw)
	}
	v := s.do("POST", "/api/auth/verify", "", map[string]any{"token": s.mailToken(email)})
	if v.code != 200 || v.str("userId") == "" {
		s.t.Fatalf("verify: %d %s", v.code, v.raw)
	}
	// The registration answer names nothing (ADR-0023); the membership does.
	var tenant string
	if err := s.db.Admin.QueryRow(s.ctx, "SELECT tenant_id FROM identity_membership WHERE user_id = $1", v.str("userId")).Scan(&tenant); err != nil {
		s.t.Fatal(err)
	}
	return tenant
}

func (s *stack) login(email string) reply {
	s.t.Helper()
	r := s.do("POST", "/api/auth/login", "", map[string]any{"email": email, "password": password})
	if r.code != 200 || r.str("token") == "" {
		s.t.Fatalf("login %s: %d %s", email, r.code, r.raw)
	}
	return r
}

// member adds a person with a role to an existing workspace, through the
// console's path, and logs them in.
func (s *stack) member(email, slug, role string) reply {
	s.t.Helper()
	if _, err := identity.CreateUser(s.ctx, s.svc, identity.CreateUserInput{Email: email, Password: password, TenantSlug: slug, TenantName: slug, Role: role}); err != nil {
		s.t.Fatal(err)
	}
	return s.login(email)
}

// operator creates the operator account once and logs it in: the manager
// realm's token, with no cookie. A super operator, the one kind that may
// impersonate.
func (s *stack) operator() reply {
	s.t.Helper()
	if _, _, err := manager.CreateOperator(s.ctx, s.svc, manager.CreateOperatorInput{Email: opEmail, Password: opPassword, DisplayName: "Support", Super: true}); err != nil {
		s.t.Fatal(err)
	}
	r := s.do("POST", "/api/manager/login", "", map[string]any{"email": opEmail, "password": opPassword})
	if r.code != 200 || r.str("token") == "" {
		s.t.Fatalf("operator login: %d %s", r.code, r.raw)
	}
	return r
}

func perms(r reply) []string {
	raw, _ := r.body["permissions"].([]any)
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		out = append(out, p.(string))
	}
	return out
}

func has(list []string, p string) bool {
	for _, x := range list {
		if x == p {
			return true
		}
	}
	return false
}

func mustContain(t *testing.T, s, sub, what string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("%s: %q not in %q", what, sub, s)
	}
}
