package security_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/gdpr"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/storage"
)

func TestTheEmailIsCiphertextAtRestAndLoginStillWorks(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	var stored, hash string
	if err := s.db.Admin.QueryRow(s.ctx, `SELECT email, email_hash FROM identity_user`).Scan(&stored, &hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "v1:") || strings.Contains(stored, "owner") || strings.Contains(strings.ToLower(stored), "example.test") {
		t.Fatalf("the address is readable on disk: %q", stored)
	}
	if hash == "" || strings.Contains(hash, "@") {
		t.Fatalf("lookup hash: %q", hash)
	}
	if r := s.login(strings.ToUpper(ownerEmail)); r.str("user", "email") != ownerEmail {
		t.Fatalf("login by the hash is case-insensitive and the wire shows the plaintext: %s", r.raw)
	}
}

func TestTheRefreshTokenIsHttpOnlySecureAndNeverInTheBody(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	r := s.login(ownerEmail)
	c := r.refresh()
	if c == nil {
		t.Fatal("no refresh cookie")
	}
	if !c.HttpOnly || !c.Secure || c.Path != "/api/auth" || c.SameSite != 3 /* Strict */ || c.MaxAge < 29*24*3600 {
		t.Fatalf("cookie attributes: %+v", c)
	}
	if strings.Contains(string(r.raw), c.Value) || strings.Contains(string(r.raw), "refresh") {
		t.Fatalf("the refresh token is in the body: %s", r.raw)
	}
	if r.str("token") == "" || r.at("expiresIn") != float64(900) {
		t.Fatalf("the access token is in the body, short-lived: %s", r.raw)
	}
	// The access token is not a cookie.
	for _, k := range r.cookies {
		if strings.Contains(k.Value, r.str("token")) {
			t.Fatal("the access token is a cookie")
		}
	}
}

func TestCredentialFailuresAreIndistinguishable(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	// An unverified account beside the verified one.
	if r := s.do("POST", "/api/auth/register", "", map[string]any{"email": "pending@example.test", "password": password, "tenantName": "Pending"}); r.code != 202 {
		t.Fatal(r.code)
	}
	var bodies []string
	for _, in := range []map[string]any{
		{"email": "nobody@example.test", "password": password},
		{"email": ownerEmail, "password": "wrong-" + password},
		{"email": "pending@example.test", "password": password},
	} {
		r := s.do("POST", "/api/auth/login", "", in)
		if r.code != 401 {
			t.Fatalf("%v: %d %s", in, r.code, r.raw)
		}
		bodies = append(bodies, r.withoutRequestID())
	}
	if bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Fatalf("refusals differ:\n%s\n%s\n%s", bodies[0], bodies[1], bodies[2])
	}
	if strings.Contains(bodies[0], "nobody") || strings.Contains(bodies[0], "unverified") {
		t.Fatalf("a refusal says why: %s", bodies[0])
	}
}

func TestForgotPasswordAnswersTheSameForUnknownAddresses(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	known := s.do("POST", "/api/auth/forgot-password", "", map[string]any{"email": ownerEmail})
	unknown := s.do("POST", "/api/auth/forgot-password", "", map[string]any{"email": "nobody@example.test"})
	if known.code != 200 || unknown.code != 200 || known.withoutRequestID() != unknown.withoutRequestID() || known.str("status") != "sent" {
		t.Fatalf("known %d %s / unknown %d %s", known.code, known.raw, unknown.code, unknown.raw)
	}
	if _, got := s.mail.Last("nobody@example.test"); got {
		t.Fatal("a mail went to an address with no account")
	}
	if _, got := s.mail.Last(ownerEmail); !got {
		t.Fatal("no reset mail to the account")
	}
}

func TestASecurityTokenIsStoredHashedWorksOnceAndOnlyForItsPurpose(t *testing.T) {
	s := newStack(t)
	r := s.do("POST", "/api/auth/register", "", map[string]any{"email": ownerEmail, "password": password, "tenantName": "Acme"})
	if r.code != 202 {
		t.Fatal(r.code)
	}
	verifyToken := s.mailToken(ownerEmail)
	var n int
	if err := s.db.Admin.QueryRow(s.ctx, `SELECT count(*) FROM identity_security_token WHERE token_hash = $1`, verifyToken).Scan(&n); err != nil || n != 0 {
		t.Fatal("the raw token is on disk")
	}
	if err := s.db.Admin.QueryRow(s.ctx, `SELECT count(*) FROM identity_security_token WHERE purpose = 'verify_email' AND length(token_hash) = 64`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("hashed verify token rows: %d %v", n, err)
	}
	// A verify token is not a reset token.
	if x := s.do("POST", "/api/auth/reset-password", "", map[string]any{"token": verifyToken, "password": "another-" + password}); x.code != 400 {
		t.Fatalf("verify token accepted for a reset: %d %s", x.code, x.raw)
	}
	if v := s.do("POST", "/api/auth/verify", "", map[string]any{"token": verifyToken}); v.code != 200 {
		t.Fatalf("verify: %d %s", v.code, v.raw)
	}
	if v := s.do("POST", "/api/auth/verify", "", map[string]any{"token": verifyToken}); v.code != 400 {
		t.Fatalf("a verify token worked twice: %d", v.code)
	}
	// The same for a reset token.
	s.do("POST", "/api/auth/forgot-password", "", map[string]any{"email": ownerEmail})
	resetToken := s.mailToken(ownerEmail)
	if x := s.do("POST", "/api/auth/reset-password", "", map[string]any{"token": resetToken, "password": "another-" + password}); x.code != 200 || x.str("status") != "reset" {
		t.Fatalf("reset: %d %s", x.code, x.raw)
	}
	if x := s.do("POST", "/api/auth/reset-password", "", map[string]any{"token": resetToken, "password": "third-" + password}); x.code != 400 {
		t.Fatalf("a reset token worked twice: %d", x.code)
	}
	if l := s.do("POST", "/api/auth/login", "", map[string]any{"email": ownerEmail, "password": "another-" + password}); l.code != 200 {
		t.Fatalf("the new password: %d %s", l.code, l.raw)
	}
}

func TestResettingAPasswordRevokesEveryExistingSession(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	one, two := s.login(ownerEmail), s.login(ownerEmail)
	s.do("POST", "/api/auth/forgot-password", "", map[string]any{"email": ownerEmail})
	if x := s.do("POST", "/api/auth/reset-password", "", map[string]any{"token": s.mailToken(ownerEmail), "password": "another-" + password}); x.code != 200 {
		t.Fatalf("reset: %d %s", x.code, x.raw)
	}
	for i, sess := range []reply{one, two} {
		if r := s.send(request{method: "POST", path: "/api/auth/refresh", cookie: sess.refresh()}); r.code != 401 {
			t.Fatalf("session %d survived the reset: %d", i+1, r.code)
		}
	}
}

func TestAnApiKeyIsStoredHashedGrantsOnlyItsSubsetAndDiesWhenRevokedOrExpired(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	owner := s.login(ownerEmail).str("token")
	r := s.do("POST", "/api/api-keys", owner, map[string]any{"name": "ci", "permissions": []string{"example.view", "api_key.view"}})
	if r.code != 201 {
		t.Fatalf("create: %d %s", r.code, r.raw)
	}
	secret := r.str("secret")
	var stored string
	if err := s.db.Admin.QueryRow(s.ctx, `SELECT key_hash FROM api_key`).Scan(&stored); err != nil || stored == secret || len(stored) != 64 {
		t.Fatalf("stored %q (%v)", stored, err)
	}
	if l := s.do("GET", "/api/api-keys", owner, nil); strings.Contains(string(l.raw), secret) || !strings.Contains(string(l.raw), `"prefix":"`+secret[:12]+`"`) {
		t.Fatalf("the list shows the secret or not the prefix: %s", l.raw)
	}
	// The key does what it was issued for and nothing more.
	if l := s.do("GET", "/api/projects", secret, nil); l.code != 200 {
		t.Fatalf("key reading projects: %d %s", l.code, l.raw)
	}
	if c := s.do("POST", "/api/projects", secret, map[string]any{"name": "x"}); c.code != 403 {
		t.Fatalf("key writing projects without example.manage: %d %s", c.code, c.raw)
	}
	if c := s.do("POST", "/api/api-keys", secret, map[string]any{"name": "x", "permissions": []string{"example.view"}}); c.code != 403 {
		t.Fatalf("key minting keys: %d", c.code)
	}
	// Revoked: dead. Expired: dead. Same answer as unknown.
	if d := s.do("DELETE", "/api/api-keys/"+r.str("id"), owner, nil); d.code != 200 {
		t.Fatalf("revoke: %d %s", d.code, d.raw)
	}
	revoked := s.do("GET", "/api/projects", secret, nil)
	exp := s.do("POST", "/api/api-keys", owner, map[string]any{"name": "old", "permissions": []string{"example.view"}, "expiresInDays": 1})
	if _, err := s.db.Admin.Exec(s.ctx, `UPDATE api_key SET expires_at = now() - interval '1 second' WHERE id = $1`, exp.str("id")); err != nil {
		t.Fatal(err)
	}
	expired := s.do("GET", "/api/projects", exp.str("secret"), nil)
	unknown := s.do("GET", "/api/projects", "sk_"+strings.Repeat("A", 43), nil)
	if revoked.code != 401 || expired.code != 401 || unknown.code != 401 || revoked.withoutRequestID() != expired.withoutRequestID() || expired.withoutRequestID() != unknown.withoutRequestID() {
		t.Fatalf("revoked %d %s\nexpired %d %s\nunknown %d %s", revoked.code, revoked.raw, expired.code, expired.raw, unknown.code, unknown.raw)
	}
}

func TestAnUploadLandsUnderItsOwnTenantAndItsUrlExpires(t *testing.T) {
	s := newStack(t)
	tid := s.register(ownerEmail, "Acme")
	owner := s.login(ownerEmail).str("token")
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("file", "secret.txt")
	part.Write([]byte("the quarterly numbers"))
	w.Close()
	r := s.send(request{method: "POST", path: "/api/attachments", authz: owner, rawBody: &buf, contentType: w.FormDataContentType()})
	if r.code != 201 {
		t.Fatalf("upload: %d %s", r.code, r.raw)
	}
	var key string
	if err := s.db.Admin.QueryRow(s.ctx, `SELECT storage_key FROM attachment`).Scan(&key); err != nil || !strings.HasPrefix(key, tid+"/") {
		t.Fatalf("storage key %q is not under the tenant %s (%v)", key, tid, err)
	}
	url := strings.TrimPrefix(r.str("url"), apiURL)
	if g := s.send(request{method: "GET", path: url}); g.code != 200 || string(g.raw) != "the quarterly numbers" {
		t.Fatalf("signed download: %d %s", g.code, g.raw)
	}
	// Without a signature, with a wrong one, and after expiry: refused alike.
	bare := url[:strings.Index(url, "?")]
	if g := s.send(request{method: "GET", path: bare}); g.code != 403 {
		t.Fatalf("unsigned download: %d", g.code)
	}
	if g := s.send(request{method: "GET", path: strings.Replace(url, "sig=", "sig=x", 1)}); g.code != 403 {
		t.Fatalf("tampered signature: %d", g.code)
	}
	expired, err := s.svc.Storage.(*storage.Memory).TemporaryURL(scope.WithTenant(s.ctx, tid), key, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if g := s.send(request{method: "GET", path: strings.TrimPrefix(expired, apiURL)}); g.code != 403 {
		t.Fatalf("expired link: %d", g.code)
	}
	// A bearer token is no substitute for the signature.
	if g := s.send(request{method: "GET", path: bare, authz: owner}); g.code != 403 {
		t.Fatalf("a session reading an unsigned link: %d", g.code)
	}
}

func TestAPersonsDataCanBeExportedAndErased(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	sess := s.login(ownerEmail)
	userID := sess.str("user", "id")
	s.do("POST", "/api/api-keys", sess.str("token"), map[string]any{"name": "ci", "permissions": []string{"example.view"}})
	walker := gdpr.NewWalker(s.svc.Audit, s.reg.GdprSubjects(s.svc)...)
	export, err := walker.Export(s.ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(export)
	mustContain(t, string(raw), ownerEmail, "export has the address")
	mustContain(t, string(raw), `"identity.account"`, "export has the account section")
	mustContain(t, string(raw), `"audit.actions"`, "export has the trail")
	if !strings.Contains(string(raw), "api_key.created") {
		t.Fatalf("the trail in the export lacks the person's actions: %s", raw)
	}
	erased, err := walker.Erase(s.ctx, userID)
	if err != nil || erased["identity"] == 0 || erased["audit"] == 0 {
		t.Fatalf("erase: %v %v", erased, err)
	}
	if l := s.do("POST", "/api/auth/login", "", map[string]any{"email": ownerEmail, "password": password}); l.code != 401 {
		t.Fatalf("an erased person can log in: %d", l.code)
	}
	if r := s.send(request{method: "POST", path: "/api/auth/refresh", cookie: sess.refresh()}); r.code != 401 {
		t.Fatalf("an erased person's session refreshes: %d", r.code)
	}
	after, _ := walker.Export(s.ctx, userID)
	if raw, _ := json.Marshal(after); strings.Contains(string(raw), ownerEmail) || strings.Contains(string(raw), userID) {
		t.Fatalf("still identifiable after erasure: %s", raw)
	}
	var actors int
	if err := s.db.Admin.QueryRow(s.ctx, `SELECT count(*) FROM audit_entry WHERE actor_id = $1`, userID).Scan(&actors); err != nil || actors != 0 {
		t.Fatalf("the trail still names the person: %d %v", actors, err)
	}
}

func TestGuessesAtOneAddressFromManyMachinesRunOutAtTen(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	guess := func(ip, pw string) reply {
		return s.send(request{method: "POST", path: "/api/auth/login", payload: map[string]any{"email": ownerEmail, "password": pw}, headers: map[string]string{"X-Forwarded-For": ip}})
	}
	for i := 1; i <= 10; i++ {
		if r := guess(fmt.Sprintf("198.51.100.%d", i), "guess-"+password); r.code != 401 {
			t.Fatalf("guess %d: %d %s", i, r.code, r.raw)
		}
	}
	// The right password from an eleventh machine: the address is spent.
	if r := guess("203.0.113.9", password); r.code != 429 || r.str("error", "code") != "rate_limited" || r.str("token") != "" || r.refresh() != nil {
		t.Fatalf("eleventh attempt: %d %s", r.code, r.raw)
	}
}
