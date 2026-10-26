package scope

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/open-enu/kernel/testdb"
)

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
)

// table creates a scoped table as the owner and two rows, one per tenant,
// then returns the application's pool.
func table(t *testing.T) *Pool {
	t.Helper()
	db := testdb.Postgres(t)
	ctx := context.Background()
	ddl := `CREATE TABLE thing (tenant_id uuid NOT NULL, id serial PRIMARY KEY, name text NOT NULL, version int NOT NULL DEFAULT 1);
` + Policy("thing", "tenant_id", "uuid")
	if _, err := db.Admin.Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Admin.Exec(ctx, "INSERT INTO thing (tenant_id, name) VALUES ($1, 'a'), ($2, 'b')", tenantA, tenantB); err != nil {
		t.Fatal(err)
	}
	p, err := Open(ctx, db.AppDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func names(t *testing.T, p *Pool, ctx context.Context) []string {
	t.Helper()
	var out []string
	err := p.Run(ctx, func(ctx context.Context, tx *Tx) error {
		rows, err := tx.Query(ctx, "SELECT name FROM thing ORDER BY name")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return err
			}
			out = append(out, n)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPolicyFailsClosedAndSeparatesTenants(t *testing.T) {
	p := table(t)
	bare := context.Background()
	if got := names(t, p, bare); len(got) != 0 {
		t.Fatalf("no tenant in scope must see NO rows, saw %v", got)
	}
	if got := names(t, p, WithTenant(bare, tenantA)); strings.Join(got, ",") != "a" {
		t.Fatalf("tenant A sees A only, got %v", got)
	}
	if got := names(t, p, WithTenant(bare, tenantB)); strings.Join(got, ",") != "b" {
		t.Fatalf("tenant B sees B only, got %v", got)
	}
	var all []string
	err := Unscoped(bare, "test: count every tenant's rows", func(ctx context.Context) error {
		all = names(t, p, ctx)
		return nil
	})
	if err != nil || strings.Join(all, ",") != "a,b" {
		t.Fatalf("bypass sees everything: %v %v", all, err)
	}
	if err := Unscoped(bare, "  ", func(context.Context) error { return nil }); err == nil {
		t.Fatal("an empty reason is refused")
	}
}

func TestSetLocalDoesNotLeakAcrossAPooledConnection(t *testing.T) {
	db := testdb.Postgres(t)
	ctx := context.Background()
	if _, err := db.Admin.Exec(ctx, `CREATE TABLE thing (tenant_id uuid NOT NULL, name text NOT NULL);
`+Policy("thing", "tenant_id", "uuid")); err != nil {
		t.Fatal(err)
	}
	_, _ = db.Admin.Exec(ctx, "INSERT INTO thing VALUES ($1, 'a'), ($2, 'b')", tenantA, tenantB)
	cfg, err := pgxpool.ParseConfig(db.AppDSN)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1 // every transaction below uses the SAME connection
	one, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	p := &Pool{Pool: one}
	if got := names(t, p, WithTenant(ctx, tenantA)); strings.Join(got, ",") != "a" {
		t.Fatal(got)
	}
	if got := names(t, p, ctx); len(got) != 0 {
		t.Fatalf("the next transaction on the same connection inherited a tenant: %v", got)
	}
	if got := names(t, p, WithTenant(ctx, tenantB)); strings.Join(got, ",") != "b" {
		t.Fatal(got)
	}
	// Even a session-level read of the setting is empty between transactions.
	var setting string
	if err := one.QueryRow(ctx, "SELECT current_setting('app.tenant_id', true)").Scan(&setting); err != nil || setting != "" {
		t.Fatalf("setting leaked to the session: %q %v", setting, err)
	}
}

func TestWritesAreCheckedAgainstTheScope(t *testing.T) {
	p := table(t)
	ctx := WithTenant(context.Background(), tenantA)
	err := p.Run(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO thing (tenant_id, name) VALUES ($1, 'smuggled')", tenantB)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Fatalf("inserting another tenant's row under A must be refused by the policy, got %v", err)
	}
	err = p.Run(ctx, func(ctx context.Context, tx *Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO thing (tenant_id, name) VALUES ($1, 'mine')", tenantA)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// An UPDATE on B's row from A's scope touches nothing: the row is invisible.
	err = p.Run(ctx, func(ctx context.Context, tx *Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE thing SET name = 'x' WHERE name = 'b'")
		if err != nil {
			return err
		}
		return CheckVersion(tag)
	})
	if !errors.Is(err, ErrStale) {
		t.Fatalf("zero rows is ErrStale, got %v", err)
	}
	if got := names(t, p, WithTenant(context.Background(), tenantB)); strings.Join(got, ",") != "b" {
		t.Fatalf("B's row untouched: %v", got)
	}
}

type rec struct{ Name string }

func (r rec) Snapshot() map[string]any { return map[string]any{"name": r.Name} }

func TestSnapshotsRideOnTheTransaction(t *testing.T) {
	p := table(t)
	ctx := WithTenant(context.Background(), tenantA)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, _, ok := tx.Snapshots(); ok {
		t.Fatal("nothing recorded yet")
	}
	tx.Update(rec{"a"}, rec{"a2"})
	before, after, ok := tx.Snapshots()
	if !ok || before["name"] != "a" || after["name"] != "a2" {
		t.Fatalf("%v %v %v", before, after, ok)
	}
	tx.Insert(rec{"n"})
	if b, a, _ := tx.Snapshots(); b != nil || a["name"] != "n" {
		t.Fatal("insert has no before")
	}
	tx.Delete(rec{"d"})
	if b, a, _ := tx.Snapshots(); a != nil || b["name"] != "d" {
		t.Fatal("delete has no after")
	}
}

func TestPolicySQL(t *testing.T) {
	sql := Policy("audit_entry", "tenant_id", "text")
	for _, want := range []string{"ENABLE ROW LEVEL SECURITY", "FORCE ROW LEVEL SECURITY", "tenant_id = current_setting('app.tenant_id', true)", "app.bypass", "WITH CHECK"} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %s in\n%s", want, sql)
		}
	}
	if !strings.Contains(Policy("t", "tenant_id", "uuid"), "NULLIF(current_setting('app.tenant_id', true), '')::uuid") {
		t.Error("uuid columns compare on uuid")
	}
}
