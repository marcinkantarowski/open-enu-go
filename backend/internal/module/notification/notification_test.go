package notification_test

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
	"github.com/open-enu/kernel/mail"
	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/module"
	knotification "github.com/open-enu/kernel/notification"
	"github.com/open-enu/kernel/outbox"
	"github.com/open-enu/kernel/platform"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"

	"github.com/open-enu/app/internal/module/identity"
	"github.com/open-enu/app/internal/module/notification"
	"github.com/open-enu/app/internal/module/tenant"
)

// The feed through real HTTP against real Postgres, with real people from
// Identity: a notification is raised through the kernel's Notifier (as any
// module would raise it) and read back as the person it was for.

const password = "correct-horse-battery-staple"

// A type some module declared. The module under test declares none of its
// own, so the catalogue is given one here, as the registry would.
var ping = knotification.Definition{Type: "demo.ping", TitleKey: "notification.demo.ping.title", BodyKey: "notification.demo.ping.body", Email: true}

type stack struct {
	t        *testing.T
	ctx      context.Context
	svc      *platform.Services
	h        http.Handler
	signer   auth.Signer
	mail     *mail.Recorder
	notifier knotification.Notifier
	tenant   string
	alice    string
	bob      string
	baseline int // mails sent while setting up, not by the test
}

// mails is what the test itself caused to be sent.
func (s *stack) mails() []mail.Message { return s.mail.Sent()[s.baseline:] }

func newStack(t *testing.T) *stack {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	reg, err := module.NewRegistry(tenant.Module, identity.Module, notification.Module)
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
	svc.NotificationDefinitions = append(reg.Notifications(), ping)
	svc.URLs = platform.URLs{App: "https://app.test", Manager: "https://manager.test", API: "https://api.test"}
	if err := reg.LoadLocales(svc.I18n); err != nil {
		t.Fatal(err)
	}
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{MaxWorkers: 1, PollInterval: time.Second}, false); err != nil {
		t.Fatal(err)
	}
	notifier, err := reg.Notifier(svc)
	if err != nil || notifier == nil {
		t.Fatalf("notifier: %v", err)
	}
	svc.Notifier = notifier
	mux := http.NewServeMux()
	reg.Mount(mux, svc)
	keys, err := reg.Keys(svc)
	if err != nil {
		t.Fatal(err)
	}
	signer := auth.Signer{Secret: []byte(platform.TestKey)}
	s := &stack{t: t, ctx: ctx, svc: svc, signer: signer, mail: svc.Mail.(*mail.Recorder), notifier: notifier,
		h: httpx.Chain(mux, httpx.RequestID, httpx.Recover, httpx.Locale(svc.I18n), auth.Middleware(signer, keys))}
	alice, err := identity.CreateUser(ctx, svc, identity.CreateUserInput{Email: "alice@example.test", Password: password, TenantSlug: "acme", TenantName: "Acme", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := identity.CreateUser(ctx, svc, identity.CreateUserInput{Email: "bob@example.test", Password: password, TenantSlug: "acme", Role: "member"})
	if err != nil {
		t.Fatal(err)
	}
	s.tenant, s.alice, s.bob = alice.TenantID, alice.UserID, bob.UserID
	s.baseline = len(s.mail.Sent())
	return s
}

func (s *stack) as(userID string) string {
	tok, err := s.signer.Sign(auth.Claims{Subject: userID, TenantID: s.tenant, Audience: auth.RealmTenant, Permissions: []string{"notification.view"}}, time.Hour)
	if err != nil {
		s.t.Fatal(err)
	}
	return tok
}

func (s *stack) scoped() context.Context { return scope.WithTenant(s.ctx, s.tenant) }

type reply struct {
	code  int
	raw   string
	body  map[string]any
	items []map[string]any
}

func (s *stack) do(method, path, authz string) reply {
	s.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer "+authz)
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	r := reply{code: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &r.body)
	if raw, ok := r.body["items"].([]any); ok {
		r.items = []map[string]any{}
		for _, it := range raw {
			r.items = append(r.items, it.(map[string]any))
		}
	}
	return r
}

func (s *stack) feed(userID string) reply {
	s.t.Helper()
	r := s.do("GET", "/api/notifications", s.as(userID))
	if r.code != 200 || r.items == nil {
		s.t.Fatalf("feed: %d %s", r.code, r.raw)
	}
	return r
}

func unread(r reply) float64 {
	m, _ := r.body["meta"].(map[string]any)
	v, _ := m["unread"].(float64)
	return v
}

func TestARowStoresTheTypeAndItsArgumentsAndTheFeedReturnsTheDeclaredKeys(t *testing.T) {
	s := newStack(t)
	if err := s.notifier.ToUser(s.scoped(), s.alice, ping.Type, map[string]any{"count": 3, "who": "Bob"}); err != nil {
		t.Fatal(err)
	}
	r := s.feed(s.alice)
	if len(r.items) != 1 || unread(r) != 1 {
		t.Fatalf("alice's feed: %s", r.raw)
	}
	it := r.items[0]
	if it["type"] != "demo.ping" || it["titleKey"] != "notification.demo.ping.title" || it["bodyKey"] != "notification.demo.ping.body" || it["read"] != false || it["readAt"] != nil || it["createdAt"] == "" {
		t.Fatalf("item: %v", it)
	}
	if ctx := it["context"].(map[string]any); ctx["count"] != 3.0 || ctx["who"] != "Bob" {
		t.Fatalf("context: %v", ctx)
	}
	// Nothing about it is a sentence: the response carries keys and
	// arguments, and the client translates.
	if strings.Contains(r.raw, "Bob pinged") {
		t.Fatal("a rendered sentence is in the feed")
	}
	// Bob's feed is Bob's.
	if b := s.feed(s.bob); len(b.items) != 0 || unread(b) != 0 {
		t.Fatalf("bob sees alice's notification: %s", b.raw)
	}
	// The declaration asked for mail: one, to Alice, translated server-side
	// with the same arguments.
	if sent := s.mails(); len(sent) != 1 || sent[0].To != "alice@example.test" || !strings.Contains(sent[0].Text, "https://app.test/notifications") || sent[0].Subject == "" {
		t.Fatalf("mail: %+v", sent)
	}
	if r := s.do("GET", "/api/notifications", ""); r.code != 401 {
		t.Fatalf("anonymous: %d", r.code)
	}
}

func TestAnUndeclaredTypeIsDroppedNotStored(t *testing.T) {
	s := newStack(t)
	if err := s.notifier.ToUser(s.scoped(), s.alice, "demo.nobody_declared_this", nil); err != nil {
		t.Fatal(err)
	}
	if r := s.feed(s.alice); len(r.items) != 0 {
		t.Fatalf("an undeclared type was stored: %s", r.raw)
	}
	if len(s.mails()) != 0 {
		t.Fatal("mail for an undeclared type")
	}
}

func TestReadStateIsPersonalAndIdempotent(t *testing.T) {
	s := newStack(t)
	for i := 0; i < 3; i++ {
		if err := s.notifier.ToUser(s.scoped(), s.alice, ping.Type, map[string]any{"count": i}); err != nil {
			t.Fatal(err)
		}
	}
	r := s.feed(s.alice)
	if len(r.items) != 3 || unread(r) != 3 || r.items[0]["context"].(map[string]any)["count"] != 2.0 {
		t.Fatalf("newest first, three unread: %s", r.raw)
	}
	id := r.items[0]["id"].(string)
	// Bob cannot mark Alice's as read - and cannot learn that it exists.
	if b := s.do("POST", "/api/notifications/"+id+"/read", s.as(s.bob)); b.code != 404 {
		t.Fatalf("bob marked alice's: %d %s", b.code, b.raw)
	}
	one := s.do("POST", "/api/notifications/"+id+"/read", s.as(s.alice))
	if one.code != 200 || one.body["status"] != "read" {
		t.Fatalf("read: %d %s", one.code, one.raw)
	}
	after := s.feed(s.alice)
	readAt := after.items[0]["readAt"]
	if unread(after) != 2 || after.items[0]["read"] != true || readAt == nil {
		t.Fatalf("after one: %s", after.raw)
	}
	// Again: same answer, same timestamp. Two tabs must not move it.
	if again := s.do("POST", "/api/notifications/"+id+"/read", s.as(s.alice)); again.code != 200 {
		t.Fatalf("read again: %d", again.code)
	}
	if s.feed(s.alice).items[0]["readAt"] != readAt {
		t.Fatal("marking read twice moved readAt")
	}
	all := s.do("POST", "/api/notifications/read-all", s.as(s.alice))
	if all.code != 200 || all.body["status"] != "read" || all.body["unread"] != 0.0 {
		t.Fatalf("read-all: %d %s", all.code, all.raw)
	}
	if f := s.feed(s.alice); unread(f) != 0 || f.items[2]["read"] != true {
		t.Fatalf("after read-all: %s", f.raw)
	}
	if r := s.do("POST", "/api/notifications/not-a-row/read", s.as(s.alice)); r.code != 404 {
		t.Fatalf("garbage id: %d", r.code)
	}
}

func TestToTenantReachesEveryMemberOnce(t *testing.T) {
	s := newStack(t)
	// Raised from a worker: no scope in the context, the tenant is a parameter.
	if err := s.notifier.ToTenant(s.ctx, s.tenant, ping.Type, map[string]any{"count": 1}); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{s.alice, s.bob} {
		if r := s.feed(u); len(r.items) != 1 || unread(r) != 1 {
			t.Fatalf("%s: %s", u, r.raw)
		}
	}
	if n := len(s.mails()); n != 2 {
		t.Fatalf("%d mails, want one per member", n)
	}
}

func TestASubjectAccessRequestCoversEveryTenantAndErasureIsCounted(t *testing.T) {
	s := newStack(t)
	// Alice in a second workspace too.
	other, err := identity.CreateUser(s.ctx, s.svc, identity.CreateUserInput{Email: "alice@example.test", Password: password, TenantSlug: "globex", TenantName: "Globex", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.notifier.ToUser(s.scoped(), s.alice, ping.Type, map[string]any{"count": 1})
	_ = s.notifier.ToUser(scope.WithTenant(s.ctx, other.TenantID), s.alice, ping.Type, map[string]any{"count": 2})
	_ = s.notifier.ToUser(s.scoped(), s.bob, ping.Type, nil)
	subject := notification.Module.Gdpr(s.svc)
	export, err := subject.ExportFor(s.ctx, s.alice)
	if err != nil || len(export["notifications"]) != 2 {
		t.Fatalf("export: %v %v", export, err)
	}
	tenants := map[any]bool{}
	for _, row := range export["notifications"] {
		tenants[row["tenantId"]] = true
	}
	if !tenants[s.tenant] || !tenants[other.TenantID] {
		t.Fatalf("export is not across tenants: %v", export)
	}
	n, err := subject.EraseFor(s.ctx, s.alice)
	if err != nil || n != 2 {
		t.Fatalf("erase: %d %v", n, err)
	}
	if r := s.feed(s.alice); len(r.items) != 0 {
		t.Fatal("alice still has notifications")
	}
	if r := s.feed(s.bob); len(r.items) != 1 {
		t.Fatal("bob's were erased too")
	}
}
