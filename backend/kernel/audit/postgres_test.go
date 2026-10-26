package audit

import (
	"context"
	"testing"

	"github.com/open-enu/kernel/migrate"
	"github.com/open-enu/kernel/scope"
	"github.com/open-enu/kernel/testdb"
)

func TestPostgresWritesAndTheTenantReadsOnlyItsOwn(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil); err != nil {
		t.Fatal(err)
	}
	pool := &scope.Pool{Pool: db.App}
	l := Postgres{Pool: pool}
	l.Record(scope.WithTenant(ctx, "t1"), Entry{Action: "demo.thing.rename", SubjectID: "s1", ActorID: "u1", TenantID: "t1", RequestID: "r1", Succeeded: true,
		Before: map[string]any{"name": "a"}, After: map[string]any{"name": "b"}, Context: map[string]any{"duration_ms": 4}})
	l.Record(ctx, Entry{Action: "manager.tenant.suspend", SubjectID: "t2", TenantID: "t2", Succeeded: false, FailureReason: "boom"})
	l.Record(ctx, Entry{Action: "system.sweep", Succeeded: true}) // no tenant at all

	count := func(ctx context.Context) int {
		var n int
		if err := pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM audit_entry").Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(scope.WithTenant(ctx, "t1")); n != 1 {
		t.Fatalf("tenant t1 sees its one entry, got %d", n)
	}
	if n := count(ctx); n != 0 {
		t.Fatalf("no scope sees nothing, got %d", n)
	}
	var all int
	_ = scope.Unscoped(ctx, "test: operator view", func(ctx context.Context) error { all = count(ctx); return nil })
	if all != 3 {
		t.Fatalf("operator sees all three, got %d", all)
	}
	var before, failure string
	var tenant *string
	err := scope.Unscoped(ctx, "test", func(ctx context.Context) error {
		return pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error {
			if err := tx.QueryRow(ctx, "SELECT before::text FROM audit_entry WHERE action = 'demo.thing.rename'").Scan(&before); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, "SELECT failure_reason FROM audit_entry WHERE action = 'manager.tenant.suspend'").Scan(&failure); err != nil {
				return err
			}
			return tx.QueryRow(ctx, "SELECT tenant_id FROM audit_entry WHERE action = 'system.sweep'").Scan(&tenant)
		})
	})
	if err != nil || before != `{"name": "a"}` || failure != "boom" || tenant != nil {
		t.Fatalf("before=%q failure=%q tenant=%v err=%v", before, failure, tenant, err)
	}
	// A broken pool never surfaces as a panic or an error to the caller.
	broken := Postgres{Pool: &scope.Pool{}}
	broken.Record(ctx, Entry{Action: "x.y"})
}
