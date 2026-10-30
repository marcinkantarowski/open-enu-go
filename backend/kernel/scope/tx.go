package scope

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the database. Every transaction it opens carries the context's
// tenant into Postgres as `SET LOCAL app.tenant_id`, where the RLS policy
// reads it (ADR-0009). LOCAL, never session-level: the connection goes back
// to the pool after the transaction and the next borrower must not inherit
// a tenant.
type Pool struct {
	*pgxpool.Pool
}

// DefaultMaxConns is the pool ceiling when the DSN does not set
// `pool_max_conns`. pgx's own default is max(4, NumCPU), and four is not
// enough here: one request can hold three connections at once - the
// command's transaction, a nested command's (Identity's register runs
// Tenant's create, which commits on its own) and a tenant-setup step's.
// Two such requests on a four-connection pool wait for each other forever.
const DefaultMaxConns = 20

// Open connects. The DSN is the application's role - NOSUPERUSER, no
// BYPASSRLS - or the policies are decoration (testdb.AppRole in tests).
// `pool_max_conns=N` in the DSN overrides DefaultMaxConns.
func Open(ctx context.Context, dsn string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("scope: %w", err)
	}
	if !strings.Contains(dsn, "pool_max_conns") {
		cfg.MaxConns = DefaultMaxConns
	}
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("scope: %w", err)
	}
	return &Pool{Pool: p}, nil
}

// Tx is a transaction with the scope set. Only a handler receives one from
// the bus; a repository takes it as a parameter. It also collects the audit
// snapshots the handler provides (ADR-0013), so the bus can write them
// without the handler knowing about the trail.
type Tx struct {
	pgx.Tx
	before, after map[string]any
	snapshotted   bool
	afterCommit   []func(context.Context) error

	// The scope this transaction (or savepoint) applied, and the one it is
	// nested in: a savepoint that switched scope puts the outer one back
	// when it is released, because RELEASE keeps what SET LOCAL did inside.
	tenant string
	bypass bool
	outer  *Tx
}

// Begin opens a transaction and sets the scope: the bypass flag under
// Unscoped, the tenant id when the context has one, nothing otherwise - and
// nothing is what makes every scoped table return no rows.
func (p *Pool) Begin(ctx context.Context) (*Tx, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("scope: begin: %w", err)
	}
	if err := setScope(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	tenant, bypass := scopeOf(ctx)
	return &Tx{Tx: tx, tenant: tenant, bypass: bypass}, nil
}

// Savepoint opens a nested transaction on the same connection, scoped by
// ctx: the command bus gives one to a command dispatched from inside
// another command's handler, so the two commit or roll back together, and
// the inner one does not wait for a second connection while the outer one
// holds the first. Rollback undoes only the savepoint's work;
// Commit releases it into the outer transaction.
func (t *Tx) Savepoint(ctx context.Context) (*Tx, error) {
	sp, err := t.Tx.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("scope: savepoint: %w", err)
	}
	tenant, bypass := scopeOf(ctx)
	child := &Tx{Tx: sp, tenant: tenant, bypass: bypass, outer: t}
	if child.tenant != t.tenant || child.bypass != t.bypass {
		if err := applyScope(ctx, sp, tenant, bypass); err != nil {
			_ = sp.Rollback(ctx)
			return nil, err
		}
	}
	return child, nil
}

// Commit commits the transaction, or releases the savepoint and restores
// the outer transaction's scope if the savepoint changed it. A rollback
// needs no restoring: ROLLBACK TO SAVEPOINT reverts SET LOCAL by itself.
func (t *Tx) Commit(ctx context.Context) error {
	if err := t.Tx.Commit(ctx); err != nil {
		return err
	}
	if o := t.outer; o != nil && (o.tenant != t.tenant || o.bypass != t.bypass) {
		return applyScope(ctx, o.Tx, o.tenant, o.bypass)
	}
	return nil
}

func (t *Tx) root() *Tx {
	for t.outer != nil {
		t = t.outer
	}
	return t
}

// Run executes fn in a scoped transaction and commits unless it returns an
// error. For the bus and for reads (Read, for a read that may join
// Consistent); a handler never calls it, it is handed the Tx.
func (p *Pool) Run(ctx context.Context, fn func(ctx context.Context, tx *Tx) error) error {
	tx, err := p.Begin(ctx)
	if err != nil {
		return err
	}
	// Deferred, not only on the error path: a panic in fn must not leave the
	// connection in the pool with a transaction open. After Commit the
	// rollback is a no-op (pgx answers ErrTxClosed, which is discarded).
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return tx.RunAfterCommit(ctx)
}

// shared is the read-only transaction Consistent puts in the context. The
// lock keeps two goroutines off its one connection; a Read that cannot take
// it - a second goroutine, or a Read nested in another - gets its own
// transaction instead of waiting.
type shared struct {
	mu   sync.Mutex
	pool *Pool
	tx   *Tx
}

type sharedKey struct{}

// Consistent runs fn with one REPEATABLE READ, READ ONLY transaction in its
// context, so that every Read inside it - from any module, through any
// contract - sees the same snapshot. For a reader that assembles several
// modules' records into one answer: read one by one, each in its own
// transaction, they can straddle another request's commit and describe a
// state that never existed. Run and the bus never join it; a write inside fn
// gets its own transaction, as it would without Consistent. Nested, the
// outer one stays.
func (p *Pool) Consistent(ctx context.Context, fn func(ctx context.Context) error) error {
	if s, ok := ctx.Value(sharedKey{}).(*shared); ok && s.pool == p {
		return fn(ctx)
	}
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("scope: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := setScope(ctx, tx); err != nil {
		return err
	}
	tenant, bypass := scopeOf(ctx)
	s := &shared{pool: p, tx: &Tx{Tx: tx, tenant: tenant, bypass: bypass}}
	if err := fn(context.WithValue(ctx, sharedKey{}, s)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Read is Run for a reader. Inside Consistent, under the scope Consistent
// was opened with, it runs fn in a savepoint of the shared transaction, so
// a failed statement costs only fn's own work; anywhere else it is Run. fn
// must not write: inside Consistent the transaction is read only and the
// write fails.
func (p *Pool) Read(ctx context.Context, fn func(ctx context.Context, tx *Tx) error) error {
	s, ok := ctx.Value(sharedKey{}).(*shared)
	if !ok || s.pool != p {
		return p.Run(ctx, fn)
	}
	if tenant, bypass := scopeOf(ctx); tenant != s.tx.tenant || bypass != s.tx.bypass || !s.mu.TryLock() {
		return p.Run(ctx, fn)
	}
	defer s.mu.Unlock()
	sp, err := s.tx.Savepoint(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = sp.Rollback(ctx) }()
	if err := fn(ctx, sp); err != nil {
		return err
	}
	return sp.Commit(ctx)
}

// scopeOf is what setScope would apply for ctx: bypass wins over a tenant.
func scopeOf(ctx context.Context) (tenant string, bypass bool) {
	if isBypass(ctx) {
		return "", true
	}
	tenant, _ = TenantID(ctx)
	return tenant, false
}

// applyScope sets both settings explicitly, for a transaction that already
// carries a scope: "no tenant" has to clear the one that is there.
func applyScope(ctx context.Context, tx pgx.Tx, tenant string, bypass bool) error {
	b := "off"
	if bypass {
		b = "on"
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.bypass', $1, true), set_config('app.tenant_id', $2, true)", b, tenant); err != nil {
		return fmt.Errorf("scope: switch scope: %w", err)
	}
	return nil
}

func setScope(ctx context.Context, tx pgx.Tx) error {
	switch tenant, has := TenantID(ctx); {
	case isBypass(ctx):
		if _, err := tx.Exec(ctx, "SELECT set_config('app.bypass', 'on', true)"); err != nil {
			return fmt.Errorf("scope: set bypass: %w", err)
		}
	case has:
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant); err != nil {
			return fmt.Errorf("scope: set tenant: %w", err)
		}
	}
	return nil
}

// Snapshotter is what an entity implements so a handler can write
// `tx.Update(old, new)` and have the audit entry carry both states. The
// entity decides what is worth keeping and what is noise or a secret.
type Snapshotter interface {
	Snapshot() map[string]any
}

// Update records the before and after state of a changed record.
func (t *Tx) Update(old, new Snapshotter) { t.Snapshot(old.Snapshot(), new.Snapshot()) }

// Insert records a created record.
func (t *Tx) Insert(new Snapshotter) { t.Snapshot(nil, new.Snapshot()) }

// Delete records a removed record.
func (t *Tx) Delete(old Snapshotter) { t.Snapshot(old.Snapshot(), nil) }

// Snapshot records raw states; Update, Insert and Delete are the usual way.
func (t *Tx) Snapshot(before, after map[string]any) {
	t.before, t.after, t.snapshotted = before, after, true
}

// Snapshots returns what the handler recorded, and whether it recorded
// anything.
func (t *Tx) Snapshots() (before, after map[string]any, ok bool) {
	return t.before, t.after, t.snapshotted
}

// AfterCommit queues fn to run once the transaction has committed, in
// order, by whoever opened it (the bus, or Run). For the side effects a
// handler must not perform inside the transaction: dropping a cache entry
// (another request would re-fill it from the uncommitted, still-old row),
// sending a mail (the link in it would point at a row a rollback removes).
// A rollback discards the queue. An error from fn is returned to the caller
// after the write is already durable, so it is reported, never retried.
// On a savepoint the function waits for the outermost commit, not for the
// savepoint's release.
func (t *Tx) AfterCommit(fn func(ctx context.Context) error) {
	r := t.root()
	r.afterCommit = append(r.afterCommit, fn)
}

// RunAfterCommit runs the queued functions. The bus calls it; a handler
// never does.
func (t *Tx) RunAfterCommit(ctx context.Context) error {
	fns := t.afterCommit
	t.afterCommit = nil
	for _, fn := range fns {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ErrStale is returned by CheckVersion: the row's version is not the one
// the caller saw. The repository turns it into httpx.Conflict with both
// versions, which needs a re-read it is the only one placed to do.
var ErrStale = errors.New("scope: stale version - the row changed since it was read")

// CheckVersion is the optimistic-lock idiom: an UPDATE with `WHERE version =
// $n` that touched no row means somebody else wrote first.
func CheckVersion(tag pgconn.CommandTag) error {
	if tag.RowsAffected() == 0 {
		return ErrStale
	}
	return nil
}
