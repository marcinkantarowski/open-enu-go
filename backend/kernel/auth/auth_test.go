package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/scope"
)

var secret = []byte("test-key-test-key-test-key-test-key-32")

func TestTokensRoundTripAndTamperingIsRefused(t *testing.T) {
	s := Signer{Secret: secret}
	tok, err := s.Sign(Claims{Subject: "u1", TenantID: "t1", Audience: RealmTenant, Permissions: []string{"x.view"}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Verify(tok)
	if err != nil || c.Subject != "u1" || c.TenantID != "t1" || !c.principal().Has("x.view") {
		t.Fatalf("verify: %+v %v", c, err)
	}
	// Three segments, with the payload changed: the MAC no longer matches.
	parts := strings.Split(tok, ".")
	parts[1] = parts[1][:len(parts[1])-2] + "AA"
	if _, err := s.Verify(strings.Join(parts, ".")); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("tampered payload accepted: %v", err)
	}
	if _, err := (Signer{Secret: []byte("other-secret-other-secret-other-secret")}).Verify(tok); err == nil {
		t.Fatal("another secret verified the token")
	}
	// alg:none, the classic.
	none := "eyJhbGciOiJub25lIn0." + parts[1] + "."
	if _, err := s.Verify(none); err == nil {
		t.Fatal("alg none accepted")
	}
	// Expired.
	past := Signer{Secret: secret, Now: func() time.Time { return time.Now().Add(-2 * time.Hour) }}
	old, _ := past.Sign(Claims{Subject: "u1", Audience: RealmTenant}, time.Hour)
	if _, err := s.Verify(old); err == nil {
		t.Fatal("expired token accepted")
	}
	// imp without act is malformed.
	bad, _ := s.Sign(Claims{Subject: "u1", Audience: RealmTenant, Impersonated: true}, time.Hour)
	if _, err := s.Verify(bad); err == nil {
		t.Fatal("imp without act accepted")
	}
	if _, err := (Signer{Secret: []byte("short")}).Sign(Claims{Subject: "u"}, time.Hour); err == nil {
		t.Fatal("a short secret signs")
	}
	if _, err := (Signer{}).Verify(tok); err == nil {
		t.Fatal("an unconfigured signer verified a token")
	}
	// No exp at all: correctly signed, but nothing here mints such a token,
	// and accepting it would make it valid forever.
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString([]byte(`{"sub":"u1","aud":"tenant"}`))
	forever := signing + "." + enc.EncodeToString(s.mac(signing))
	if _, err := s.Verify(forever); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("a token without exp was accepted: %v", err)
	}
}

type brokenKeys struct{}

func (brokenKeys) Resolve(context.Context, string) (Principal, bool, error) {
	return Principal{}, false, errors.New("dial tcp 10.0.0.5:5432: connect: connection refused")
}

func TestAResolverThatCannotAnswerIsAServerErrorNotABadKey(t *testing.T) {
	h := Middleware(Signer{Secret: secret}, brokenKeys{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("the request went through")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer sk_whatever")
	h.ServeHTTP(rec, req)
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestMiddlewareAuthenticatesBothCredentialsAndEntersTheScope(t *testing.T) {
	s := Signer{Secret: secret}
	keys := MemoryKeys{}
	key := keys.Add("sk_test-key", Principal{Subject: "k1", TenantID: "t1", Realm: RealmTenant, Permissions: []string{"x.view"}})
	var seen Principal
	var tenant string
	h := Middleware(s, keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = From(r.Context())
		tenant, _ = scope.TenantID(r.Context())
		w.WriteHeader(204)
	}))
	call := func(authz string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/x", nil)
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		h.ServeHTTP(rec, req)
		return rec
	}
	tok, _ := s.Sign(Claims{Subject: "u1", TenantID: "t1", Audience: RealmTenant, Impersonated: true, Actor: &ActorInfo{Subject: "op1", Realm: RealmManager}}, time.Hour)
	if rec := call("Bearer " + tok); rec.Code != 204 || seen.Subject != "u1" || seen.Impersonator != "op1" || tenant != "t1" {
		t.Fatalf("jwt: %d %+v tenant=%q", rec.Code, seen, tenant)
	}
	if rec := call("Bearer " + key); rec.Code != 204 || seen.Subject != "k1" || !seen.APIKey || tenant != "t1" {
		t.Fatalf("key: %d %+v", rec.Code, seen)
	}
	if rec := call(""); rec.Code != 204 || seen.Subject != "" {
		t.Fatalf("anonymous passes through: %d %+v", rec.Code, seen)
	}
	for _, bad := range []string{"Bearer sk_unknown", "Bearer not.a.jwt", "Basic abc"} {
		rec := call(bad)
		var body map[string]map[string]any
		if rec.Code != 401 || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body["error"]["code"] != "unauthenticated" {
			t.Fatalf("%q: %d %s", bad, rec.Code, rec.Body.String())
		}
	}
}

func TestRequireAndTheImpersonationGuard(t *testing.T) {
	ctx := context.Background()
	if _, err := Require(ctx, "x.view"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("anonymous: %v", err)
	}
	// The other realm's token is refused like no token: the realm check is
	// what keeps an operator off the tenant API (ADR-0019).
	op := WithPrincipal(ctx, Principal{Subject: "op", Realm: RealmManager, Permissions: []string{"x.view"}})
	if _, err := Require(op, "x.view"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("operator on the tenant realm: %v", err)
	}
	if _, err := RequireOperator(op); err != nil {
		t.Fatalf("operator on the manager realm: %v", err)
	}
	ctx = WithPrincipal(ctx, Principal{Subject: "u1", TenantID: "t1", Realm: RealmTenant, Permissions: []string{"x.view"}})
	if _, err := RequireOperator(ctx); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("tenant user on the manager realm: %v", err)
	}
	if _, err := Require(ctx, "x.manage"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("missing permission: %v", err)
	}
	if _, err := Require(ctx, "x.view"); err != nil {
		t.Fatal(err)
	}
	rec := &audit.Recorder{}
	g := Guard{Audit: rec}
	if err := g.DenyImpersonated(ctx, "x.rename"); err != nil || len(rec.Entries()) != 0 {
		t.Fatalf("own session is not denied: %v", err)
	}
	imp := WithPrincipal(ctx, Principal{Subject: "u1", TenantID: "t1", Impersonator: "op1"})
	err := g.DenyImpersonated(imp, "x.rename")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("impersonated: %v", err)
	}
	e := rec.Entries()
	if len(e) != 1 || e[0].ActorID != "u1" || e[0].OnBehalfOf != "op1" || e[0].Succeeded || e[0].Action != "x.rename" {
		t.Fatalf("the refusal names both identities: %+v", e)
	}
	if a := (Actor{}).Actor(imp); a.ID != "u1" || a.OnBehalfOf != "op1" {
		t.Fatalf("actor: %+v", a)
	}
}
