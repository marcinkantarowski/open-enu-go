package identity_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/open-enu/kernel/crypto"
	"github.com/open-enu/kernel/httpx"

	"github.com/open-enu/app/internal/module/identity"
	identityc "github.com/open-enu/app/internal/module/identity/contract"
)

// written is what a module's endpoint sends when it answers with a started
// session - the recorder a real response writer would have been.
func written(t *testing.T, started identityc.Started) reply {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := started.Write(rec); err != nil {
		t.Fatal(err)
	}
	r := reply{code: rec.Code, cookies: rec.Result().Cookies(), header: rec.Header()}
	if err := json.Unmarshal(rec.Body.Bytes(), &r.body); err != nil {
		t.Fatalf("not JSON: %s", rec.Body.String())
	}
	return r
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// A module that authenticated a person by other means - single sign-on, a
// mailed link, a replaced system's credentials - asks Identity for an
// ordinary session. It is a login's answer, made by the same code, and its
// cookie renews it like any other.
func TestAStartedSessionIsALoginsAnswerAndRenewsLikeOne(t *testing.T) {
	s := newStack(t)
	primary := s.register("Primary")
	second, err := identity.CreateUser(s.ctx, s.svc, identity.CreateUserInput{Email: email, Password: "ignored-the-account-exists", TenantSlug: "second", TenantName: "Second", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	login := s.login()
	userID := login.str("user", "id")
	minter := identityc.NewMinter(s.svc)

	started, err := minter.StartSession(s.ctx, userID, second.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	r := written(t, started)
	if r.code != 200 || r.str("token") == "" || r.str("user", "id") != userID || r.str("tenantId") != second.TenantID || r.str("role") != "admin" {
		t.Fatalf("started session: %d %v", r.code, r.body)
	}
	if !slices.Equal(keys(r.body), keys(login.body)) {
		t.Fatalf("not a login's answer: %v against %v", keys(r.body), keys(login.body))
	}
	if c := r.refresh(); c == nil || !c.HttpOnly || c.Value == "" || c.MaxAge <= 0 {
		t.Fatalf("no refresh cookie a login would set: %+v", c)
	}
	renewed := s.do("POST", "/api/auth/refresh", "", nil, r.refresh())
	if renewed.code != 200 || renewed.str("tenantId") != second.TenantID || renewed.refresh() == nil || renewed.refresh().Value == r.refresh().Value {
		t.Fatalf("the started session does not renew: %d %v", renewed.code, renewed.body)
	}

	// No workspace named: where a login lands, the one they own.
	best, err := minter.StartSession(s.ctx, userID, "")
	if err != nil {
		t.Fatal(err)
	}
	if b := written(t, best); b.code != 200 || b.str("tenantId") != primary || b.str("role") != "owner" {
		t.Fatalf("best workspace: %d %v", b.code, b.body)
	}

	// An unverified person: registered, never confirmed.
	if pending := s.post("/api/auth/register", map[string]any{"email": "pending@example.test", "password": password, "tenantName": "Pending"}); pending.code != 202 {
		t.Fatalf("register: %d %v", pending.code, pending.body)
	}
	var pendingID string
	if err := s.db.Admin.QueryRow(s.ctx, "SELECT id FROM identity_user WHERE status = 'unverified'").Scan(&pendingID); err != nil {
		t.Fatal(err)
	}

	// Every refusal is the login's own 401, and nothing to write.
	for name, try := range map[string][2]string{
		"an unknown person":                 {uuid.NewString(), ""},
		"an id that is not one":             {"not-an-id", ""},
		"an unverified person":              {pendingID, ""},
		"a workspace they do not belong to": {userID, uuid.NewString()},
	} {
		_, err := minter.StartSession(s.ctx, try[0], try[1])
		var he *httpx.Error
		if !errors.As(err, &he) || he.Status != 401 || he.Code != "unauthenticated" {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A verifier made at another cost still lets its owner in, and is replaced
// by the login that proved the password (ADR-0022).
func TestALoginReplacesAVerifierOfAnotherCost(t *testing.T) {
	s := newStack(t)
	s.register("Acme")
	old, err := crypto.PasswordHasher{Cost: crypto.PasswordCost{MemoryKiB: 16, Time: 2, Threads: 1}}.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Admin.Exec(s.ctx, "UPDATE identity_user SET password = $1", old); err != nil {
		t.Fatal(err)
	}
	stored := func() string {
		var v string
		if err := s.db.Admin.QueryRow(s.ctx, "SELECT password FROM identity_user").Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if l := s.post("/api/auth/login", map[string]any{"email": email, "password": "wrong-horse-battery-staple"}); l.code != 401 || stored() != old {
		t.Fatalf("a wrong password changes nothing: %d", l.code)
	}
	s.login()
	if v := stored(); v == old || !strings.HasPrefix(v, "$argon2id$v=19$m=8,t=1,p=1$") {
		t.Fatalf("the verifier was not replaced at the platform's cost: %s", v)
	}
	s.login() // and the new one checks
}
