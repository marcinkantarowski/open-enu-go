package security_test

import (
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/auth"

	auditc "github.com/open-enu/app/internal/module/audit/contract"
)

// Two realms verify against one key, so isolation rests on the `aud` claim
// (ADR-0019) and on the one designed crossing between them (ADR-0020).

func TestEachTokenWorksOnItsOwnRealmAndIsRefusedOnTheOther(t *testing.T) {
	s := newStack(t)
	s.register(ownerEmail, "Acme")
	owner, op := s.login(ownerEmail).str("token"), s.operator().str("token")
	if r := s.do("GET", "/api/projects", owner, nil); r.code != 200 {
		t.Fatalf("owner on the tenant realm: %d", r.code)
	}
	if r := s.do("GET", "/api/manager/tenants", op, nil); r.code != 200 {
		t.Fatalf("operator on the manager realm: %d", r.code)
	}
	refused := []reply{
		s.do("GET", "/api/projects", op, nil),
		s.do("GET", "/api/api-keys", op, nil),
		s.do("GET", "/api/settings", op, nil),
		s.do("GET", "/api/profile", op, nil), // a 404 here would say the realms share a key
		s.do("GET", "/api/manager/tenants", owner, nil),
		s.do("GET", "/api/manager/settings", owner, nil),
		s.do("GET", "/api/manager/audit", owner, nil),
	}
	anonymous := s.do("GET", "/api/projects", "", nil)
	for i, r := range refused {
		if r.code != 401 || r.withoutRequestID() != anonymous.withoutRequestID() {
			t.Fatalf("crossing %d: %d %s (anonymous: %s)", i, r.code, r.raw, anonymous.raw)
		}
	}
}

func TestATenantTokenSharingAnOperatorsAddressIsStillATenantToken(t *testing.T) {
	// The same address in both tables is two accounts: a login on one realm
	// never yields the other's token.
	s := newStack(t)
	s.register(opEmail, "Acme")
	s.operator()
	tenantSide := s.do("POST", "/api/auth/login", "", map[string]any{"email": opEmail, "password": password})
	if tenantSide.code != 200 {
		t.Fatalf("tenant login: %d", tenantSide.code)
	}
	if r := s.do("GET", "/api/manager/tenants", tenantSide.str("token"), nil); r.code != 401 {
		t.Fatalf("a tenant token with an operator's address on the manager realm: %d", r.code)
	}
	if r := s.do("POST", "/api/manager/login", "", map[string]any{"email": opEmail, "password": password}); r.code != 401 {
		t.Fatalf("the tenant password opened the operator account: %d", r.code)
	}
}

func TestAForgedAudienceDoesNotHelp(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	other := auth.Signer{Secret: []byte(strings.Repeat("not-the-secret-", 3))}
	forgedOp, _ := other.Sign(auth.Claims{Subject: "x", Audience: auth.RealmManager}, time.Hour)
	if r := s.do("GET", "/api/manager/tenants", forgedOp, nil); r.code != 401 {
		t.Fatalf("forged operator token: %d", r.code)
	}
	// A real tenant token re-labelled: the signature no longer matches.
	owner := s.login(ownerEmail).str("token")
	parts := strings.Split(owner, ".")
	payload := strings.Replace(parts[1], "tenant", "manager", 1)
	if r := s.do("GET", "/api/manager/tenants", parts[0]+"."+payload+"."+parts[2], nil); r.code != 401 {
		t.Fatalf("re-labelled token: %d", r.code)
	}
	// Minted correctly for the realm by the right key: nothing else is checked
	// but the realm, so this proves the audience is the whole decision.
	minted, _ := s.svc.Signer.Sign(auth.Claims{Subject: "op", Audience: auth.RealmManager}, time.Hour)
	if r := s.do("GET", "/api/manager/tenants", minted, nil); r.code != 200 {
		t.Fatalf("a correctly minted operator token: %d %s", r.code, r.raw)
	}
	if r := s.do("GET", "/api/manager/tenants/"+a, minted, nil); r.code != 200 {
		t.Fatalf("detail: %d", r.code)
	}
}

func TestAnImpersonationTokenActsInTheTenantRealmIsMarkedAndCannotActDestructivelyOrRenew(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	s.register("other@example.test", "Beta")
	sess := s.login(ownerEmail)
	op := s.operator().str("token")
	// Turn the flagged route on for Acme, so the guard - not the flag - is what refuses.
	if r := s.do("PUT", "/api/manager/tenants/"+a+"/settings/example.archive", op, map[string]any{"value": true}); r.code != 200 {
		t.Fatalf("flag on: %d %s", r.code, r.raw)
	}
	imp := s.do("POST", "/api/manager/tenants/"+a+"/users/"+sess.str("user", "id")+"/impersonate", op, nil)
	if imp.code != 200 || imp.body["impersonating"] != true || imp.refresh() != nil {
		t.Fatalf("impersonate: %d %s cookie %v", imp.code, imp.raw, imp.refresh())
	}
	token := imp.str("token")
	claims, err := s.svc.Signer.Verify(token)
	if err != nil || claims.Audience != auth.RealmTenant || !claims.Impersonated || claims.Actor == nil || claims.Actor.Realm != auth.RealmManager {
		t.Fatalf("claims: %+v %v", claims, err)
	}
	// Looks like the person on the tenant realm...
	if r := s.do("GET", "/api/projects", token, nil); r.code != 200 {
		t.Fatalf("impersonated read: %d %s", r.code, r.raw)
	}
	if r := s.do("GET", "/api/manager/tenants", token, nil); r.code != 401 {
		t.Fatalf("impersonation token on the manager realm: %d", r.code)
	}
	// ...but may not act destructively, and the refusal names both identities.
	if r := s.do("POST", "/api/projects/archive", token, nil); r.code != 403 {
		t.Fatalf("a guarded action under impersonation: %d %s", r.code, r.raw)
	}
	if r := s.do("POST", "/api/projects/archive", sess.str("token"), nil); r.code != 202 {
		t.Fatalf("the same action by the person: %d %s", r.code, r.raw)
	}
	var denied bool
	entries, err := auditc.NewReader(s.svc).Recent(s.ctx, nil, 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Action == "example.projects.archive_requested" && !e.Succeeded && e.OnBehalfOfID != nil && *e.OnBehalfOfID == claims.Actor.Subject && e.ActorID != nil && *e.ActorID == sess.str("user", "id") {
			denied = true
		}
	}
	if !denied {
		t.Fatal("the refusal is not in the trail with both identities")
	}
	// It cannot renew itself: there is no cookie, and the person's own cookie
	// is not it. Holding the impersonation token changes nothing - the answer
	// is "nothing to restore", with no token and no cookie in it.
	if r := s.send(request{method: "POST", path: "/api/auth/refresh", authz: token}); r.code != 204 || len(r.raw) != 0 || len(r.cookies) != 0 {
		t.Fatalf("refresh without a cookie: %d %s %v", r.code, r.raw, r.cookies)
	}
	// Membership is required: Beta's owner is not in Acme.
	beta := s.login("other@example.test")
	if r := s.do("POST", "/api/manager/tenants/"+a+"/users/"+beta.str("user", "id")+"/impersonate", op, nil); r.code != 400 {
		t.Fatalf("impersonating a non-member: %d %s", r.code, r.raw)
	}
}

func TestAFlagFlippedForOneTenantDoesNotAffectAnother(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	s.register("other@example.test", "Beta")
	op := s.operator().str("token")
	ownerA, ownerB := s.login(ownerEmail).str("token"), s.login("other@example.test").str("token")
	// Off by default: the route does not exist, for both.
	for _, tok := range []string{ownerA, ownerB} {
		if r := s.do("POST", "/api/projects/archive", tok, nil); r.code != 404 {
			t.Fatalf("archive while off: %d", r.code)
		}
	}
	if r := s.do("PUT", "/api/manager/tenants/"+a+"/settings/example.archive", op, map[string]any{"value": true}); r.code != 200 {
		t.Fatalf("flag on: %d %s", r.code, r.raw)
	}
	if r := s.do("POST", "/api/projects/archive", ownerA, nil); r.code != 202 {
		t.Fatalf("A after the switch: %d %s", r.code, r.raw)
	}
	if r := s.do("POST", "/api/projects/archive", ownerB, nil); r.code != 404 {
		t.Fatalf("B after A's switch: %d", r.code)
	}
	// And back off, biting on the next request.
	s.do("PUT", "/api/manager/tenants/"+a+"/settings/example.archive", op, map[string]any{"value": false})
	if r := s.do("POST", "/api/projects/archive", ownerA, nil); r.code != 404 {
		t.Fatalf("A after switching off: %d", r.code)
	}
}
