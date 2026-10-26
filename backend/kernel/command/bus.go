package command

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/open-enu/kernel/audit"
	"github.com/open-enu/kernel/httpx"
	"github.com/open-enu/kernel/scope"
)

// Bus is the one way a write happens. Dispatch opens the scoped transaction,
// hands the handler the *scope.Tx, commits or rolls back, and records an
// audit entry either way - including the snapshots the handler left on the
// transaction with tx.Update(old, new). A handler cannot forget to audit,
// because it is not the handler doing it.
//
// Synchronous on purpose: the caller usually needs the result. Work that
// belongs in the background is a job the handler enqueues through the outbox,
// in the same transaction.
//
// A command dispatched from inside a handler (Identity's register creating
// the workspace through Tenant's own command) joins the running transaction
// as a savepoint: both commit or neither does, and the inner one does not
// wait for a second connection while the outer one holds the first - which,
// with every request doing it, is how a pool runs dry and stays dry. Its
// audit entry is written when the outermost command finishes: a success
// only if that commit happened, a failure either way.
type Bus struct {
	pool     *scope.Pool
	audit    audit.Logger
	actor    audit.ActorProvider
	handlers map[reflect.Type]func(ctx context.Context, tx *scope.Tx, cmd Command) (any, error)
	now      func() time.Time
}

// NewBus wires the bus to the database and the trail.
func NewBus(pool *scope.Pool, log audit.Logger, actor audit.ActorProvider) *Bus {
	if actor == nil {
		actor = audit.NoActor{}
	}
	return &Bus{pool: pool, audit: log, actor: actor, handlers: map[reflect.Type]func(context.Context, *scope.Tx, Command) (any, error){}, now: time.Now}
}

// Register binds a command type to its handler. One handler per command;
// a second registration is a programming error and panics at start-up.
func Register[C Command, R any](b *Bus, h func(ctx context.Context, tx *scope.Tx, cmd C) (R, error)) {
	var zero C
	t := reflect.TypeOf(zero)
	if t == nil {
		panic("command: Register needs a concrete command type, not an interface")
	}
	if _, dup := b.handlers[t]; dup {
		panic(fmt.Sprintf("command: %s already has a handler", t))
	}
	b.handlers[t] = func(ctx context.Context, tx *scope.Tx, cmd Command) (any, error) {
		return h(ctx, tx, cmd.(C))
	}
}

// running is the outermost command's transaction, on the context its
// handler receives, with the entries of the commands nested in it.
type running struct {
	tx      *scope.Tx
	entries []audit.Entry
}

type runningKey struct{}

type commandKey struct{}

// Running returns the audit action of the command whose handler is running
// on ctx, "" outside one - for log lines (kernel/logctx).
func Running(ctx context.Context) string {
	c, _ := ctx.Value(commandKey{}).(string)
	return c
}

// Dispatch runs the command's handler in a scoped transaction and records
// the entry. The handler's error is returned as is, after the rollback and
// after the failure was recorded.
func (b *Bus) Dispatch(ctx context.Context, cmd Command) (any, error) {
	h, ok := b.handlers[reflect.TypeOf(cmd)]
	if !ok {
		return nil, fmt.Errorf("command: no handler registered for %T - command.Register it where the module mounts", cmd)
	}
	if run, ok := ctx.Value(runningKey{}).(*running); ok {
		return b.nested(ctx, run, h, cmd)
	}
	started := b.now()
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	// Deferred so a panicking handler cannot leak a connection with a
	// transaction open (httpx.Recover turns the panic into a 500, and the
	// next borrower of that connection would otherwise inherit the mess).
	// After a successful Commit this is a no-op.
	defer func() { _ = tx.Rollback(ctx) }()
	run := &running{tx: tx}
	result, err := h(context.WithValue(context.WithValue(ctx, runningKey{}, run), commandKey{}, cmd.AuditAction()), tx, cmd)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		for _, e := range run.entries {
			if !e.Succeeded {
				b.audit.Record(ctx, e) // a refusal happened, whatever followed it
			}
		}
		b.audit.Record(ctx, b.entry(ctx, cmd, started, nil, nil).Failed(err.Error()))
		return nil, err
	}
	for _, e := range run.entries {
		b.audit.Record(ctx, e)
	}
	before, after, _ := tx.Snapshots()
	b.audit.Record(ctx, b.entry(ctx, cmd, started, before, after))
	// What the handler queued for after the commit: cache invalidation,
	// mail. The write is durable and recorded by now, so a failure here is
	// returned as such, not as a failed command.
	if err := tx.RunAfterCommit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

// nested runs a command dispatched from inside a handler in a savepoint of
// the running transaction, and keeps its entry for the outermost Dispatch
// to write once it knows whether anything was committed.
func (b *Bus) nested(ctx context.Context, run *running, h func(context.Context, *scope.Tx, Command) (any, error), cmd Command) (any, error) {
	started := b.now()
	sp, err := run.tx.Savepoint(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sp.Rollback(ctx) }() // a panic, or nothing after Commit
	result, err := h(context.WithValue(ctx, commandKey{}, cmd.AuditAction()), sp, cmd)
	if err == nil {
		err = sp.Commit(ctx)
	}
	if err != nil {
		_ = sp.Rollback(ctx)
		run.entries = append(run.entries, b.entry(ctx, cmd, started, nil, nil).Failed(err.Error()))
		return nil, err
	}
	before, after, _ := sp.Snapshots()
	run.entries = append(run.entries, b.entry(ctx, cmd, started, before, after))
	return result, nil
}

// Dispatch is the typed form: the handler's result as R.
func Dispatch[R any](ctx context.Context, b *Bus, cmd Command) (R, error) {
	var zero R
	v, err := b.Dispatch(ctx, cmd)
	if err != nil {
		return zero, err
	}
	if v == nil {
		return zero, nil
	}
	r, ok := v.(R)
	if !ok {
		return zero, fmt.Errorf("command: %T's handler returned %T, the caller wanted %T", cmd, v, zero)
	}
	return r, nil
}

func (b *Bus) entry(ctx context.Context, cmd Command, started time.Time, before, after map[string]any) audit.Entry {
	actor := b.actor.Actor(ctx)
	// Ambient scope first, and the command's own answer when there is none:
	// an operator acting from the manager realm has no tenant in scope, and
	// an entry written without one disappears from the per-tenant trail.
	tenant, _ := scope.TenantID(ctx)
	if tenant == "" {
		if tt, ok := cmd.(TenantTargeted); ok {
			tenant = tt.AuditTenantID()
		}
	}
	return audit.Entry{
		Action: cmd.AuditAction(), SubjectID: cmd.AuditSubjectID(),
		ActorID: actor.ID, OnBehalfOf: actor.OnBehalfOf, TenantID: tenant,
		RequestID: httpx.RequestIDFrom(ctx), Succeeded: true, Before: before, After: after,
		Context: map[string]any{"duration_ms": b.now().Sub(started).Milliseconds()}, At: b.now(),
	}
}
