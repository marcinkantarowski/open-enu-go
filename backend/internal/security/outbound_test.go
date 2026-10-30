package security_test

import (
	"context"
	"strings"
	"testing"

	"github.com/open-enu/kernel/scope"
)

// The outbound surfaces, asked the suite's questions: a webhook secret is one
// tenant's and ciphertext at rest; a feed is one person's; a search excerpt
// is content and needs the type's permission; none of it answers an
// operator's token.

func TestAWebhookSecretIsOneTenantsAndIsCiphertextAtRest(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	s.register("other@example.test", "Beta")
	ownerA, ownerB := s.login(ownerEmail), s.login("other@example.test")
	r := s.do("POST", "/api/webhooks", ownerA.str("token"), map[string]any{"url": "https://hooks.example.test/acme", "events": []string{"example.project.created"}})
	if r.code != 201 || !strings.HasPrefix(r.str("secret"), "whsec_") {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	secret := r.str("secret")
	// The list never carries it, and B's list never carries the endpoint.
	if l := s.do("GET", "/api/webhooks", ownerA.str("token"), nil); l.code != 200 || strings.Contains(string(l.raw), "whsec_") || !strings.Contains(string(l.raw), "hooks.example.test/acme") {
		t.Fatalf("A's list: %d %s", l.code, l.raw)
	}
	if l := s.do("GET", "/api/webhooks", ownerB.str("token"), nil); l.code != 200 || strings.Contains(string(l.raw), "acme") {
		t.Fatalf("B's list: %d %s", l.code, l.raw)
	}
	if d := s.do("DELETE", "/api/webhooks/"+r.str("id"), ownerB.str("token"), nil); d.code != 404 {
		t.Fatalf("B deactivated A's endpoint: %d %s", d.code, d.raw)
	}
	// At rest: ciphertext, under A's key - readable in A's scope, not as a
	// row B could ever decrypt, and never the plaintext.
	var stored string
	if err := s.svc.DB.Run(scope.WithTenant(s.ctx, a), func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, `SELECT secret FROM webhook_endpoint`).Scan(&stored)
	}); err != nil {
		t.Fatal(err)
	}
	if stored == secret || strings.Contains(stored, "whsec_") {
		t.Fatal("the secret is stored in clear")
	}
	if plain, err := s.svc.Crypto.Decrypt(stored, a); err != nil || plain != secret {
		t.Fatalf("not decryptable under A's key: %v", err)
	}
	if _, err := s.svc.Crypto.Decrypt(stored, "22222222-2222-2222-2222-222222222222"); err == nil {
		t.Fatal("decryptable under another tenant's key")
	}
	// A member sees the endpoints; only a manager may add one. An
	// impersonating operator may look and may not add.
	member := s.member("member@example.test", s.slugOf(a), "member").str("token")
	if l := s.do("GET", "/api/webhooks", member, nil); l.code != 200 || !strings.Contains(string(l.raw), "hooks.example.test/acme") {
		t.Fatalf("member list: %d %s", l.code, l.raw)
	}
	if c := s.do("POST", "/api/webhooks", member, map[string]any{"url": "https://x.example.test", "events": []string{"a.b"}}); c.code != 403 {
		t.Fatalf("member created: %d %s", c.code, c.raw)
	}
	op := s.operator().str("token")
	imp := s.do("POST", "/api/manager/tenants/"+a+"/users/"+ownerA.str("user", "id")+"/impersonate", op, nil).str("token")
	if l := s.do("GET", "/api/webhooks", imp, nil); l.code != 200 {
		t.Fatalf("impersonated list: %d %s", l.code, l.raw)
	}
	if c := s.do("POST", "/api/webhooks", imp, map[string]any{"url": "https://x.example.test", "events": []string{"a.b"}}); c.code != 403 {
		t.Fatalf("impersonator created an endpoint: %d %s", c.code, c.raw)
	}
	if l := s.do("GET", "/api/webhooks", op, nil); l.code != 401 {
		t.Fatalf("an operator's token on the tenant realm: %d", l.code)
	}
}

func TestAFeedIsOnePersonsAndNotAnOperators(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	s.register("other@example.test", "Beta")
	ownerA, ownerB := s.login(ownerEmail), s.login("other@example.test")
	// Raised as the webhook worker raises it: to the workspace, by id.
	if err := s.svc.Notifier.ToTenant(s.ctx, a, "webhook.delivery_failed", map[string]any{"event": "example.project.created", "url": "https://hooks.example.test/acme", "attempts": 5}); err != nil {
		t.Fatal(err)
	}
	fa := s.do("GET", "/api/notifications", ownerA.str("token"), nil)
	if fa.code != 200 || !strings.Contains(string(fa.raw), `"titleKey":"notification.webhook.delivery_failed.title"`) || !strings.Contains(string(fa.raw), `"unread":1`) {
		t.Fatalf("A's feed: %d %s", fa.code, fa.raw)
	}
	// The row is a key and arguments, not the sentence the catalogue holds.
	if strings.Contains(string(fa.raw), "stopped responding") {
		t.Fatalf("a rendered sentence in the feed: %s", fa.raw)
	}
	if fb := s.do("GET", "/api/notifications", ownerB.str("token"), nil); fb.code != 200 || !strings.Contains(string(fb.raw), `"unread":0`) || strings.Contains(string(fb.raw), "acme") {
		t.Fatalf("B's feed: %d %s", fb.code, fb.raw)
	}
	items, _ := fa.body["items"].([]any)
	id, _ := items[0].(map[string]any)["id"].(string)
	if r := s.do("POST", "/api/notifications/"+id+"/read", ownerB.str("token"), nil); r.code != 404 {
		t.Fatalf("B marked A's read: %d %s", r.code, r.raw)
	}
	// The mail went to A's owner, in A's owner's language, and to nobody in B.
	m, ok := s.mail.Last(ownerEmail)
	if !ok || !strings.Contains(m.Text, "hooks.example.test/acme") {
		t.Fatalf("mail to A's owner: %+v", m)
	}
	if m, _ := s.mail.Last("other@example.test"); strings.Contains(m.Text, "hooks.example.test") {
		t.Fatal("B's owner was mailed about A's endpoint")
	}
	if r := s.do("GET", "/api/notifications", s.operator().str("token"), nil); r.code != 401 {
		t.Fatalf("an operator read a feed: %d", r.code)
	}
}

func TestASearchExcerptNeedsTheTypesPermissionAndStaysInItsTenant(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	s.register("other@example.test", "Beta")
	ownerA, ownerB := s.login(ownerEmail), s.login("other@example.test")
	if r := s.do("POST", "/api/projects", ownerA.str("token"), map[string]any{"name": "Apollo launch plan"}); r.code != 201 {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	if r := s.do("GET", "/api/search?q=apollo", ownerA.str("token"), nil); r.code != 200 || !strings.Contains(string(r.raw), `"type":"example.project"`) || !strings.Contains(string(r.raw), "Apollo") {
		t.Fatalf("A finds: %d %s", r.code, r.raw)
	}
	if r := s.do("GET", "/api/search?q=apollo", ownerB.str("token"), nil); r.code != 200 || !strings.Contains(string(r.raw), `"total":0`) || strings.Contains(string(r.raw), "Apollo") {
		t.Fatalf("B found A's project: %d %s", r.code, r.raw)
	}
	// A member holds example.view and finds it too; an API key scoped away
	// from the type does not, though it is in the same tenant.
	member := s.member("member@example.test", s.slugOf(a), "member").str("token")
	if r := s.do("GET", "/api/search?q=apollo", member, nil); r.code != 200 || !strings.Contains(string(r.raw), "Apollo") {
		t.Fatalf("member: %d %s", r.code, r.raw)
	}
	key := s.do("POST", "/api/api-keys", ownerA.str("token"), map[string]any{"name": "audit only", "permissions": []string{"audit.view"}})
	if key.code != 201 {
		t.Fatalf("key: %d %s", key.code, key.raw)
	}
	if r := s.do("GET", "/api/search?q=apollo", key.str("secret"), nil); r.code != 200 || strings.Contains(string(r.raw), "Apollo") || !strings.Contains(string(r.raw), `"total":0`) {
		t.Fatalf("a key without example.view got an excerpt: %d %s", r.code, r.raw)
	}
	if r := s.do("GET", "/api/search?q=apollo", s.operator().str("token"), nil); r.code != 401 {
		t.Fatalf("an operator searched: %d", r.code)
	}
}

// slugOf is a workspace's slug: registration makes it unique with a suffix,
// so a member added "to acme" by slug would otherwise land in a new tenant.
func (s *stack) slugOf(tenantID string) (slug string) {
	s.t.Helper()
	err := scope.Unscoped(s.ctx, "test: reading a workspace's slug", func(ctx context.Context) error {
		return s.svc.DB.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			return tx.QueryRow(ctx, `SELECT slug FROM tenant WHERE id = $1`, tenantID).Scan(&slug)
		})
	})
	if err != nil {
		s.t.Fatal(err)
	}
	return slug
}

func TestAWebhookCannotPointIntoOurOwnNetwork(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	owner := s.login(ownerEmail).str("token")
	for _, url := range []string{
		"https://127.0.0.1/hook", "https://10.0.0.5/hook", "https://172.17.0.1/hook", "https://192.168.0.1/hook",
		"https://169.254.169.254/latest/meta-data/", "https://[::1]/hook", "https://[fd00::5]/hook", "https://localhost/hook",
		"https://postgres.example.test:5432/", "http://hooks.example.test/hook",
	} {
		r := s.do("POST", "/api/webhooks", owner, map[string]any{"url": url, "events": []string{"example.project.created"}})
		if r.code != 400 || r.str("error", "code") != "bad_request" || strings.HasPrefix(r.str("error", "message"), "webhook.error") {
			t.Fatalf("%s: %d %s", url, r.code, r.raw)
		}
	}
	if l := s.do("GET", "/api/webhooks", owner, nil); strings.Contains(string(l.raw), "hook") {
		t.Fatalf("a refused endpoint was stored: %s", l.raw)
	}
}
