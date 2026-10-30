package webhook_test

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
	"github.com/open-enu/kernel/module"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/webhook"
)

// The API, through real HTTP against real Postgres. The delivery path is
// internal/delivery_test.go, which needs a client that trusts the receiver.

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

type stack struct {
	t      *testing.T
	ctx    context.Context
	db     *testdb.DB
	svc    *platform.Services
	h      http.Handler
	signer auth.Signer
}

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil, migrate.Source{Name: "webhook", FS: webhook.Module.Migrations}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.Actor = auth.Actor{}
	reg, err := module.NewRegistry(webhook.Module)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.LoadLocales(svc.I18n); err != nil {
		t.Fatal(err)
	}
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 1, PollInterval: time.Second}, false); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	reg.Mount(mux, svc)
	signer := auth.Signer{Secret: []byte(platform.TestKey)}
	return &stack{t: t, ctx: ctx, db: db, svc: svc, signer: signer, h: httpx.Chain(mux, httpx.RequestID, httpx.Recover, httpx.Locale(svc.I18n), auth.Middleware(signer, auth.MemoryKeys{}))}
}

func (s *stack) token(tenant string, perms ...string) string {
	tok, err := s.signer.Sign(auth.Claims{Subject: "user-1", TenantID: tenant, Audience: auth.RealmTenant, Permissions: perms}, time.Hour)
	if err != nil {
		s.t.Fatal(err)
	}
	return tok
}

func (s *stack) manager() string { return s.token(tenantA, "webhook.view", "webhook.manage") }
func (s *stack) viewer() string  { return s.token(tenantA, "webhook.view") }
func (s *stack) impersonated() string {
	tok, _ := s.signer.Sign(auth.Claims{Subject: "user-1", TenantID: tenantA, Audience: auth.RealmTenant, Permissions: []string{"webhook.view", "webhook.manage"},
		Impersonated: true, Actor: &auth.ActorInfo{Subject: "op-1", Realm: auth.RealmManager}}, time.Hour)
	return tok
}

type reply struct {
	code int
	raw  string
	body map[string]any
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
func (r reply) items() []any { v, _ := r.body["items"].([]any); return v }

func (s *stack) do(method, path, authz string, payload any) reply {
	s.t.Helper()
	return s.doIn("", method, path, authz, payload)
}

// doIn is do with an Accept-Language.
func (s *stack) doIn(locale, method, path, authz string, payload any) reply {
	s.t.Helper()
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authz)
	if locale != "" {
		req.Header.Set("Accept-Language", locale)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	r := reply{code: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &r.body)
	return r
}

func (s *stack) create(authz, url string, events ...string) reply {
	s.t.Helper()
	return s.do("POST", "/api/webhooks", authz, map[string]any{"url": url, "events": events})
}

func TestTheSecretIsShownOnceAndNeverAgain(t *testing.T) {
	s := newStack(t)
	r := s.create(s.manager(), "https://example.test/hook", "example.project.created")
	if r.code != 201 || !strings.HasPrefix(r.str("secret"), "whsec_") || len(r.str("secret")) < 40 || r.str("warning") == "" || r.str("warning") == "webhook.secret.warning" {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	if r.str("url") != "https://example.test/hook" || r.at("active") != true || r.at("version") != 1.0 {
		t.Fatalf("endpoint: %s", r.raw)
	}
	list := s.do("GET", "/api/webhooks", s.viewer(), nil)
	if list.code != 200 || len(list.items()) != 1 {
		t.Fatalf("list: %d %s", list.code, list.raw)
	}
	if _, present := list.items()[0].(map[string]any)["secret"]; present || strings.Contains(list.raw, "whsec_") {
		t.Fatalf("the secret is in the list: %s", list.raw)
	}
	// Nor in the audit trail: a snapshot is read by more people than the row.
	for _, e := range s.svc.Audit.(*audit.Recorder).Entries() {
		if b, _ := json.Marshal(e); strings.Contains(string(b), "whsec_") {
			t.Fatalf("the secret is in the trail: %s", b)
		}
	}
	// Nor in clear in the table.
	var stored string
	if err := s.svc.DB.Run(scope.WithTenant(s.ctx, tenantA), func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, `SELECT secret FROM webhook_endpoint`).Scan(&stored)
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "whsec_") || stored == r.str("secret") {
		t.Fatal("the secret is stored in clear")
	}
	if len(s.do("GET", "/api/webhooks", s.token(tenantB, "webhook.view"), nil).items()) != 0 {
		t.Fatal("tenant B sees tenant A's endpoint")
	}
}

func TestAnEndpointMustBeHTTPSAndSubscribeToSomething(t *testing.T) {
	s := newStack(t)
	if r := s.create(s.manager(), "http://example.test/hook", "example.project.created"); r.code != 400 || r.str("error", "message") == "" || r.str("error", "message") == "webhook.error.https_required" {
		t.Fatalf("http: %d %s", r.code, r.raw)
	}
	if r := s.create(s.manager(), "https://example.test/hook"); r.code != 400 {
		t.Fatalf("no events: %d %s", r.code, r.raw)
	}
	if r := s.create(s.manager(), "https://example.test/hook", "  "); r.code != 400 {
		t.Fatalf("blank event: %d %s", r.code, r.raw)
	}
	if r := s.create(s.viewer(), "https://example.test/hook", "example.project.created"); r.code != 403 {
		t.Fatalf("viewer created an endpoint: %d %s", r.code, r.raw)
	}
	if r := s.create(s.impersonated(), "https://example.test/hook", "example.project.created"); r.code != 403 {
		t.Fatalf("an impersonating operator created an endpoint: %d %s", r.code, r.raw)
	}
	if r := s.create("", "https://example.test/hook", "example.project.created"); r.code != 401 {
		t.Fatalf("anonymous: %d", r.code)
	}
}

func TestDeactivationKeepsTheRowAndTheLog(t *testing.T) {
	s := newStack(t)
	id := s.create(s.manager(), "https://example.test/hook", "example.project.created").str("id")
	if r := s.do("DELETE", "/api/webhooks/"+id, s.viewer(), nil); r.code != 403 {
		t.Fatalf("viewer deactivated: %d", r.code)
	}
	if r := s.do("DELETE", "/api/webhooks/"+id, s.impersonated(), nil); r.code != 403 {
		t.Fatalf("impersonator deactivated: %d", r.code)
	}
	if r := s.do("DELETE", "/api/webhooks/"+id, s.token(tenantB, "webhook.view", "webhook.manage"), nil); r.code != 404 {
		t.Fatalf("tenant B deactivated A's endpoint: %d", r.code)
	}
	r := s.do("DELETE", "/api/webhooks/"+id, s.manager(), nil)
	if r.code != 200 || r.at("active") != false || r.at("version") != 2.0 || r.str("id") != id {
		t.Fatalf("deactivate: %d %s", r.code, r.raw)
	}
	// Still listed, inactive: the log points at it.
	list := s.do("GET", "/api/webhooks", s.viewer(), nil)
	if len(list.items()) != 1 || list.items()[0].(map[string]any)["active"] != false {
		t.Fatalf("list after deactivate: %s", list.raw)
	}
	// The trail: the creation, the deactivation, and the impersonator's two
	// refused attempts - a refusal is an entry too.
	actions := map[string]int{}
	for _, e := range s.svc.Audit.(*audit.Recorder).Entries() {
		actions[e.Action]++
	}
	failed := 0
	for _, e := range s.svc.Audit.(*audit.Recorder).Entries() {
		if e.Action == "webhook.endpoint.deactivated" && !e.Succeeded {
			failed++
		}
	}
	if actions["webhook.endpoint.created"] != 1 || actions["webhook.endpoint.deactivated"] != 3 || failed != 2 {
		t.Fatalf("trail: %v, %d refused", actions, failed)
	}
	if r := s.do("DELETE", "/api/webhooks/not-a-uuid", s.manager(), nil); r.code != 404 {
		t.Fatalf("garbage id: %d", r.code)
	}
}

func TestTheDeliveryLogIsReadableAndRedeliverNeedsARow(t *testing.T) {
	s := newStack(t)
	id := s.create(s.manager(), "https://example.test/hook", "example.project.created").str("id")
	r := s.do("GET", "/api/webhooks/deliveries?endpointId="+id, s.viewer(), nil)
	if r.code != 200 || r.items() == nil || len(r.items()) != 0 {
		t.Fatalf("empty log: %d %s", r.code, r.raw)
	}
	if r := s.do("GET", "/api/webhooks/deliveries", "", nil); r.code != 401 {
		t.Fatalf("anonymous log: %d", r.code)
	}
	if r := s.do("POST", "/api/webhooks/deliveries/0192f000-0000-7000-8000-0000000000ff/redeliver", s.manager(), nil); r.code != 404 {
		t.Fatalf("redeliver nothing: %d %s", r.code, r.raw)
	}
	if r := s.do("POST", "/api/webhooks/deliveries/0192f000-0000-7000-8000-0000000000ff/redeliver", s.viewer(), nil); r.code != 403 {
		t.Fatalf("viewer redelivered: %d", r.code)
	}
}

func TestAnEndpointInsideOurOwnNetworkIsRefusedInEitherLanguage(t *testing.T) {
	s := newStack(t)
	want := map[string]string{"en": "must be a public address", "pl": "musi być adresem publicznym"}
	for _, url := range []string{"https://127.0.0.1/hook", "https://10.0.0.8/hook", "https://169.254.169.254/latest/meta-data/", "https://[::1]/hook", "https://localhost/hook", "https://hooks.example.test:5432/"} {
		for locale, text := range want {
			r := s.doIn(locale, "POST", "/api/webhooks", s.manager(), map[string]any{"url": url, "events": []string{"example.project.created"}})
			if r.code != 400 || !strings.Contains(r.str("error", "message"), text) {
				t.Fatalf("%s (%s): %d %s", url, locale, r.code, r.raw)
			}
		}
	}
	if l := s.do("GET", "/api/webhooks", s.viewer(), nil); len(l.items()) != 0 {
		t.Fatalf("a refused endpoint was stored: %s", l.raw)
	}
}
