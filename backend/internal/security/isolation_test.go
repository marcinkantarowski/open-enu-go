package security_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/open-enu/kernel/auth"
	"github.com/open-enu/kernel/scope"
)

// Every tenant-scoped table is under the same row-level policy: what a
// tenant cannot see, it cannot see by any route - not through the API, not
// against the raw table as the application role.

func TestATenantSeesOnlyItsOwnRowsOnEveryNewTable(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	b := s.register("other@example.test", "Beta")
	ownerA, ownerB := s.login(ownerEmail).str("token"), s.login("other@example.test").str("token")
	keyA := s.do("POST", "/api/api-keys", ownerA, map[string]any{"name": "a", "permissions": []string{"example.view"}})
	keyB := s.do("POST", "/api/api-keys", ownerB, map[string]any{"name": "b", "permissions": []string{"example.view"}})
	if keyA.code != 201 || keyB.code != 201 {
		t.Fatal(keyA.code, keyB.code)
	}
	// Through the API: each lists one, its own; the other's exact id is 404.
	for _, c := range []struct {
		token, mine, theirs string
	}{{ownerA, keyA.str("id"), keyB.str("id")}, {ownerB, keyB.str("id"), keyA.str("id")}} {
		l := s.do("GET", "/api/api-keys", c.token, nil)
		items, _ := l.body["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["id"] != c.mine {
			t.Fatalf("list: %s", l.raw)
		}
		if d := s.do("DELETE", "/api/api-keys/"+c.theirs, c.token, nil); d.code != 404 {
			t.Fatalf("revoking the other's key by exact id: %d", d.code)
		}
	}
	if l := s.do("GET", "/api/audit", ownerA, nil); strings.Contains(string(l.raw), b) || !strings.Contains(string(l.raw), a) {
		t.Fatalf("A's trail mentions B: %s", l.raw)
	}
	// Against the raw tables as the application role, with and without a tenant.
	pool := s.svc.DB
	count := func(ctx context.Context, table string) int {
		var n int
		err := pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			// Tenantless rows (the trail's system entries) are nobody's and
			// are left out of the arithmetic.
			return tx.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE tenant_id IS NOT NULL`).Scan(&n)
		})
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		return n
	}
	for _, table := range []string{"api_key", "audit_entry"} {
		inA, inB, bare := count(scope.WithTenant(s.ctx, a), table), count(scope.WithTenant(s.ctx, b), table), count(s.ctx, table)
		if inA == 0 || inB == 0 || bare != 0 {
			t.Fatalf("%s: A %d, B %d, no scope %d - the policy must fail closed", table, inA, inB, bare)
		}
		if inA+inB != count(unscopedCtx(t, s.ctx), table) {
			t.Fatalf("%s: scoped counts do not add up to the whole", table)
		}
	}
	// A write with the wrong tenant is refused by the policy, not by code.
	err := pool.Run(scope.WithTenant(s.ctx, a), func(ctx context.Context, tx *scope.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO api_key (id, tenant_id, name, key_hash, key_prefix) VALUES (gen_random_uuid(), $1, 'smuggled', 'h', 'p')`, b)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Fatalf("a row for B written from A's scope: %v", err)
	}
}

func unscopedCtx(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	var out context.Context
	if err := scope.Unscoped(ctx, "test: the whole table", func(ctx context.Context) error { out = ctx; return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUnscopedRequiresAReasonAndTheScopeIsRestoredAfterItFails(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	inA := scope.WithTenant(s.ctx, a)
	if err := scope.Unscoped(inA, "   ", func(context.Context) error { return nil }); err == nil {
		t.Fatal("a blank reason was accepted")
	}
	boom := errors.New("boom")
	if err := scope.Unscoped(inA, "test: a failing block", func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("the error did not surface: %v", err)
	}
	if tid, ok := scope.TenantID(inA); !ok || tid != a {
		t.Fatalf("the scope changed under the caller: %q %v", tid, ok)
	}
}

func TestAJobCarriesItsTenantAndAnUnstampedOneSeesNothing(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	// Example's archive job runs in the tenant it was enqueued from; a
	// progress row it writes lands under that tenant and nowhere else.
	owner := s.login(ownerEmail).str("token")
	if r := s.do("POST", "/api/projects", owner, map[string]any{"name": "To archive"}); r.code != 201 {
		t.Fatalf("project: %d %s", r.code, r.raw)
	}
	var rows int
	err := s.svc.DB.Run(scope.WithTenant(s.ctx, a), func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM example_project`).Scan(&rows)
	})
	if err != nil || rows != 1 {
		t.Fatalf("in scope: %d %v", rows, err)
	}
	if err := s.svc.DB.Run(s.ctx, func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM example_project`).Scan(&rows)
	}); err != nil || rows != 0 {
		t.Fatalf("unstamped: %d %v", rows, err)
	}
}

func TestForgedExpiredAndWronglySignedTokensAreRefusedAlike(t *testing.T) {
	s := newStack(t)
	a := s.register(ownerEmail, "Acme")
	good := s.login(ownerEmail).str("token")
	other := auth.Signer{Secret: []byte(strings.Repeat("not-the-secret-", 3))}
	forged, _ := other.Sign(auth.Claims{Subject: "u", TenantID: a, Audience: auth.RealmTenant, Permissions: []string{"audit.view"}}, time.Hour)
	expired, _ := s.svc.Signer.Sign(auth.Claims{Subject: "u", TenantID: a, Audience: auth.RealmTenant, Permissions: []string{"audit.view"}}, -time.Minute)
	parts := strings.Split(good, ".")
	tampered := parts[0] + "." + parts[1][:len(parts[1])-2] + "AA." + parts[2]
	var bodies []string
	for name, tok := range map[string]string{"forged": forged, "expired": expired, "tampered": tampered, "garbage": "not.a.token", "none": "eyJhbGciOiJub25lIn0.e30."} {
		r := s.do("GET", "/api/audit", tok, nil)
		if r.code != 401 {
			t.Fatalf("%s: %d %s", name, r.code, r.raw)
		}
		bodies = append(bodies, r.withoutRequestID())
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Fatalf("refusals differ: %v", bodies)
		}
	}
	if r := s.do("GET", "/api/audit", good, nil); r.code != 200 {
		t.Fatalf("the real token: %d", r.code)
	}
}
