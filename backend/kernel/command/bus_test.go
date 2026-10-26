package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"
)

const tenantA = "11111111-1111-1111-1111-111111111111"

type rename struct {
	ID   int
	Name string
	fail bool
}

func (r rename) AuditAction() string    { return "demo.thing.rename" }
func (r rename) AuditSubjectID() string { return "1" }

type suspend struct{ Tenant string }

func (s suspend) AuditAction() string    { return "manager.tenant.suspend" }
func (s suspend) AuditSubjectID() string { return s.Tenant }
func (s suspend) AuditTenantID() string  { return s.Tenant }

type thing struct{ Name string }

func (t thing) Snapshot() map[string]any { return map[string]any{"name": t.Name} }

type actor struct{}

func (actor) Actor(context.Context) audit.Actor { return audit.Actor{ID: "u1", OnBehalfOf: "op1"} }

func setup(t *testing.T) (*Bus, *scope.Pool, *audit.Recorder) {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := db.Admin.Exec(ctx, `CREATE TABLE thing (tenant_id uuid NOT NULL, id int PRIMARY KEY, name text NOT NULL);
`+scope.Policy("thing", "tenant_id", "uuid")); err != nil {
		t.Fatal(err)
	}
	_, _ = db.Admin.Exec(ctx, "INSERT INTO thing VALUES ($1, 1, 'old')", tenantA)
	pool := &scope.Pool{Pool: db.App}
	trail := &audit.Recorder{}
	bus := NewBus(pool, trail, actor{})
	Register(bus, func(ctx context.Context, tx *scope.Tx, c rename) (string, error) {
		var old string
		if err := tx.QueryRow(ctx, "SELECT name FROM thing WHERE id = $1", c.ID).Scan(&old); err != nil {
			return "", err
		}
		if _, err := tx.Exec(ctx, "UPDATE thing SET name = $1 WHERE id = $2", c.Name, c.ID); err != nil {
			return "", err
		}
		tx.Update(thing{old}, thing{c.Name})
		if c.fail {
			return "", errors.New("handler refused")
		}
		return "renamed", nil
	})
	Register(bus, func(ctx context.Context, tx *scope.Tx, c suspend) (struct{}, error) { return struct{}{}, nil })
	return bus, pool, trail
}

func name(t *testing.T, pool *scope.Pool) string {
	t.Helper()
	var n string
	err := pool.Run(scope.WithTenant(context.Background(), tenantA), func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, "SELECT name FROM thing WHERE id = 1").Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDispatchCommitsAndAuditsWithSnapshots(t *testing.T) {
	bus, pool, trail := setup(t)
	ctx := scope.WithTenant(context.Background(), tenantA)
	out, err := Dispatch[string](ctx, bus, rename{ID: 1, Name: "new"})
	if err != nil || out != "renamed" {
		t.Fatalf("%q %v", out, err)
	}
	if name(t, pool) != "new" {
		t.Fatal("committed")
	}
	e := trail.Entries()
	if len(e) != 1 {
		t.Fatalf("one entry, got %d", len(e))
	}
	if e[0].Action != "demo.thing.rename" || !e[0].Succeeded || e[0].TenantID != tenantA || e[0].ActorID != "u1" || e[0].OnBehalfOf != "op1" ||
		e[0].Before["name"] != "old" || e[0].After["name"] != "new" || e[0].Context["duration_ms"] == nil {
		t.Fatalf("entry %+v", e[0])
	}
}

func TestDispatchRollsBackAndRecordsTheFailure(t *testing.T) {
	bus, pool, trail := setup(t)
	ctx := scope.WithTenant(context.Background(), tenantA)
	_, err := bus.Dispatch(ctx, rename{ID: 1, Name: "new", fail: true})
	if err == nil || err.Error() != "handler refused" {
		t.Fatalf("the handler's own error comes back: %v", err)
	}
	if name(t, pool) != "old" {
		t.Fatal("rolled back")
	}
	e := trail.Entries()
	if len(e) != 1 || e[0].Succeeded || e[0].FailureReason != "handler refused" || e[0].Before != nil {
		t.Fatalf("a failed entry with the reason and no snapshots: %+v", e)
	}
	// Outside the tenant's scope the row is invisible: the handler's read fails.
	_, err = bus.Dispatch(context.Background(), rename{ID: 1, Name: "x"})
	if err == nil || !strings.Contains(err.Error(), "no rows") {
		t.Fatalf("no scope, no row: %v", err)
	}
}

type explode struct{}

func (explode) AuditAction() string    { return "demo.thing.explode" }
func (explode) AuditSubjectID() string { return "" }

func TestAPanickingHandlerDoesNotKeepItsConnection(t *testing.T) {
	bus, pool, _ := setup(t)
	Register(bus, func(ctx context.Context, tx *scope.Tx, _ explode) (struct{}, error) {
		if _, err := tx.Exec(ctx, "UPDATE thing SET name = 'half' WHERE id = 1"); err != nil {
			return struct{}{}, err
		}
		panic("handler bug")
	})
	ctx := scope.WithTenant(context.Background(), tenantA)
	func() {
		defer func() { _ = recover() }() // httpx.Recover's job in the server
		_, _ = bus.Dispatch(ctx, explode{})
	}()
	if n := pool.Stat().AcquiredConns(); n != 0 {
		t.Fatalf("%d connection(s) still held after the panic", n)
	}
	if name(t, pool) != "old" {
		t.Fatal("the half-done write was committed")
	}
}

type remember struct {
	Name string
	fail bool
}

func (remember) AuditAction() string    { return "demo.thing.remember" }
func (remember) AuditSubjectID() string { return "1" }

func TestAfterCommitRunsOnceTheWriteIsVisibleAndNeverOnARollback(t *testing.T) {
	bus, pool, _ := setup(t)
	var seen []string
	Register(bus, func(ctx context.Context, tx *scope.Tx, c remember) (struct{}, error) {
		if _, err := tx.Exec(ctx, "UPDATE thing SET name = $1 WHERE id = 1", c.Name); err != nil {
			return struct{}{}, err
		}
		tx.AfterCommit(func(context.Context) error {
			// Another connection's read: what a request re-filling a cache
			// would see at this moment.
			seen = append(seen, name(t, pool))
			return nil
		})
		if c.fail {
			return struct{}{}, errors.New("refused")
		}
		return struct{}{}, nil
	})
	ctx := scope.WithTenant(context.Background(), tenantA)
	if _, err := bus.Dispatch(ctx, remember{Name: "fresh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := bus.Dispatch(ctx, remember{Name: "never", fail: true}); err == nil {
		t.Fatal("the failing command succeeded")
	}
	if len(seen) != 1 || seen[0] != "fresh" {
		t.Fatalf("after-commit saw %v, want exactly [fresh]", seen)
	}
}

type outer struct {
	Name     string
	inner    Command
	fail     bool
	tolerate bool // carry on when the inner command fails
}

func (outer) AuditAction() string    { return "demo.thing.outer" }
func (outer) AuditSubjectID() string { return "1" }

type inner struct {
	Name string
	fail bool
}

func (inner) AuditAction() string    { return "demo.thing.inner" }
func (inner) AuditSubjectID() string { return "2" }

const tenantB = "22222222-2222-2222-2222-222222222222"

func nestedBus(t *testing.T) (*Bus, *scope.Pool, *audit.Recorder, *int32) {
	t.Helper()
	bus, pool, trail := setup(t)
	var held int32
	Register(bus, func(ctx context.Context, tx *scope.Tx, c outer) (struct{}, error) {
		if _, err := tx.Exec(ctx, "UPDATE thing SET name = $1 WHERE id = 1", c.Name); err != nil {
			return struct{}{}, err
		}
		if _, err := bus.Dispatch(ctx, c.inner); err != nil && !c.tolerate {
			return struct{}{}, err
		}
		// The outer scope is back after the inner command ran in another one.
		var n string
		if err := tx.QueryRow(ctx, "SELECT name FROM thing WHERE id = 1").Scan(&n); err != nil {
			return struct{}{}, err
		}
		if c.fail {
			return struct{}{}, errors.New("outer refused")
		}
		return struct{}{}, nil
	})
	Register(bus, func(ctx context.Context, tx *scope.Tx, c inner) (struct{}, error) {
		held = pool.Stat().AcquiredConns()
		tid, _ := scope.TenantID(ctx)
		if _, err := tx.Exec(ctx, "INSERT INTO thing VALUES ($1, 2, $2) ON CONFLICT (id) DO UPDATE SET name = $2", tid, c.Name); err != nil {
			return struct{}{}, err
		}
		tx.Insert(thing{c.Name})
		if c.fail {
			return struct{}{}, errors.New("inner refused")
		}
		return struct{}{}, nil
	})
	return bus, pool, trail, &held
}

func row2(t *testing.T, pool *scope.Pool, tenant string) string {
	t.Helper()
	var n string
	err := pool.Run(scope.WithTenant(context.Background(), tenant), func(ctx context.Context, tx *scope.Tx) error {
		return tx.QueryRow(ctx, "SELECT coalesce((SELECT name FROM thing WHERE id = 2), '')").Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func actions(trail *audit.Recorder) []string {
	var out []string
	for _, e := range trail.Entries() {
		mark := "ok"
		if !e.Succeeded {
			mark = "failed"
		}
		out = append(out, e.Action+":"+mark)
	}
	return out
}

func TestANestedCommandJoinsTheTransactionOnTheSameConnection(t *testing.T) {
	bus, pool, trail, held := nestedBus(t)
	ctx := scope.WithTenant(context.Background(), tenantA)
	if _, err := bus.Dispatch(ctx, outer{Name: "o", inner: inner{Name: "i"}}); err != nil {
		t.Fatal(err)
	}
	if *held != 1 {
		t.Fatalf("the inner command ran with %d connections held, want 1 (the outer one)", *held)
	}
	if name(t, pool) != "o" || row2(t, pool, tenantA) != "i" {
		t.Fatal("both writes committed")
	}
	got := strings.Join(actions(trail), " ")
	if got != "demo.thing.inner:ok demo.thing.outer:ok" {
		t.Fatalf("trail: %s", got)
	}
	if e := trail.Entries()[0]; e.After["name"] != "i" || e.Before != nil {
		t.Fatalf("the inner entry keeps its own snapshot: %+v", e)
	}
}

func TestAnOuterFailureTakesTheNestedWriteWithIt(t *testing.T) {
	bus, pool, trail, _ := nestedBus(t)
	ctx := scope.WithTenant(context.Background(), tenantA)
	if _, err := bus.Dispatch(ctx, outer{Name: "o", inner: inner{Name: "i"}, fail: true}); err == nil {
		t.Fatal("outer succeeded")
	}
	if name(t, pool) != "old" || row2(t, pool, tenantA) != "" {
		t.Fatal("a write survived the outer rollback")
	}
	// No success is claimed for work that was rolled back.
	if got := strings.Join(actions(trail), " "); got != "demo.thing.outer:failed" {
		t.Fatalf("trail: %s", got)
	}
}

func TestAFailedNestedCommandRollsBackOnlyItsOwnWork(t *testing.T) {
	bus, pool, trail, _ := nestedBus(t)
	ctx := scope.WithTenant(context.Background(), tenantA)
	if _, err := bus.Dispatch(ctx, outer{Name: "o", inner: inner{Name: "i", fail: true}, tolerate: true}); err != nil {
		t.Fatal(err)
	}
	if name(t, pool) != "o" || row2(t, pool, tenantA) != "" {
		t.Fatalf("outer %q, inner %q: the savepoint should have taken only the inner write", name(t, pool), row2(t, pool, tenantA))
	}
	if got := strings.Join(actions(trail), " "); got != "demo.thing.inner:failed demo.thing.outer:ok" {
		t.Fatalf("trail: %s", got)
	}
}

func TestANestedCommandRunsInItsOwnScopeAndTheOuterOneComesBack(t *testing.T) {
	bus, pool, _, _ := nestedBus(t)
	Register(bus, func(ctx context.Context, tx *scope.Tx, c crossTenant) (struct{}, error) {
		if _, err := bus.Dispatch(scope.WithTenant(ctx, tenantB), inner{Name: "b"}); err != nil {
			return struct{}{}, err
		}
		// Back in tenant A: B's row is invisible again, A's is visible.
		var n int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM thing").Scan(&n); err != nil {
			return struct{}{}, err
		}
		if n != 1 {
			return struct{}{}, errors.New("the outer scope was not restored")
		}
		return struct{}{}, nil
	})
	if _, err := bus.Dispatch(scope.WithTenant(context.Background(), tenantA), crossTenant{}); err != nil {
		t.Fatal(err)
	}
	if row2(t, pool, tenantB) != "b" || row2(t, pool, tenantA) != "" {
		t.Fatal("the nested write landed in the wrong tenant")
	}
}

type crossTenant struct{}

func (crossTenant) AuditAction() string    { return "demo.thing.cross" }
func (crossTenant) AuditSubjectID() string { return "" }

func TestTenantTargetedAndRegistrationRules(t *testing.T) {
	bus, _, trail := setup(t)
	if _, err := bus.Dispatch(context.Background(), suspend{Tenant: "t9"}); err != nil {
		t.Fatal(err)
	}
	if e := trail.Entries(); len(e) != 1 || e[0].TenantID != "t9" {
		t.Fatalf("an operator's command names its tenant: %+v", e)
	}
	type unknown struct{ rename }
	if _, err := bus.Dispatch(context.Background(), unknown{}); err == nil || !strings.Contains(err.Error(), "no handler registered") {
		t.Fatalf("got %v", err)
	}
	if _, err := Dispatch[int](scope.WithTenant(context.Background(), tenantA), bus, rename{ID: 1, Name: "n"}); err == nil || !strings.Contains(err.Error(), "wanted int") {
		t.Fatalf("typed dispatch names the mismatch: %v", err)
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "already has a handler") {
			t.Fatalf("double registration panics: %v", r)
		}
	}()
	Register(bus, func(context.Context, *scope.Tx, rename) (int, error) { return 0, nil })
}

type parent struct{}

func (parent) AuditAction() string    { return "demo.thing.parent" }
func (parent) AuditSubjectID() string { return "1" }

type child struct{}

func (child) AuditAction() string    { return "demo.thing.child" }
func (child) AuditSubjectID() string { return "2" }

// A handler's log lines name the command running (kernel/logctx): its own,
// a nested one's while that runs, and nothing outside the bus.
func TestAHandlerKnowsWhichCommandIsRunning(t *testing.T) {
	bus, _, _ := setup(t)
	var seen []string
	Register(bus, func(ctx context.Context, tx *scope.Tx, _ parent) (struct{}, error) {
		seen = append(seen, Running(ctx))
		_, err := bus.Dispatch(ctx, child{})
		seen = append(seen, Running(ctx))
		return struct{}{}, err
	})
	Register(bus, func(ctx context.Context, tx *scope.Tx, _ child) (struct{}, error) {
		seen = append(seen, Running(ctx))
		return struct{}{}, nil
	})
	ctx := scope.WithTenant(context.Background(), tenantA)
	if _, err := bus.Dispatch(ctx, parent{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(seen, " "); got != "demo.thing.parent demo.thing.child demo.thing.parent" || Running(ctx) != "" {
		t.Fatalf("running: %q, outside: %q", got, Running(ctx))
	}
}
