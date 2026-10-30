# Commands and audit

Every state change is a command and a handler; every command leaves an audit entry,
success or failure, with the before and after state the handler provided
([ADR-0013](../adr/0013-command-bus-with-explicit-snapshots.md)). The handler cannot forget to audit,
because it is not the handler doing it.

## The shape

```go
// internal/command.go - the intent, as data
type renameProject struct{ ID, Name string; IfMatch int; Locked bool }

func (renameProject) AuditAction() string      { return "example.project.renamed" }
func (c renameProject) AuditSubjectID() string { return c.ID }

// internal/handler.go - the ONLY file that receives *scope.Tx from the bus
func (h handlers) rename(ctx context.Context, tx *scope.Tx, c renameProject) (project, error) {
	old, err := h.repo.get(ctx, tx, c.ID)                      // repository.go runs the SQL
	if err != nil { return project{}, notFound(err) }
	if !canEdit(old) { return project{}, httpx.Forbidden(keyArchived) }   // the voter
	if c.Locked && c.IfMatch != old.Version { return project{}, conflict(old, c.IfMatch) }
	updated := old
	updated.Name, updated.Version = c.Name, old.Version+1
	if err := h.repo.rename(ctx, tx, old.Version, updated); err != nil { ... ErrStale -> conflict }
	tx.Update(old, updated)                                   // the two snapshots, one line
	return updated, nil
}

// internal/mount.go - registration
command.Register(svc.Bus, h.rename)

// internal/api.go - the controller dispatches and maps
p, err := command.Dispatch[project](ctx, a.bus, renameProject{ID: req.Id, Name: name, IfMatch: v, Locked: ok})
```

`httpx.IfMatch` reads only the strong ETag `httpx.ETag` writes (`"3"`); absent and `*` are
"no lock". A present If-Match of any other shape - `W/"3"`, a bare `3`, a list - never reaches
the handler: `httpx.ValidIfMatch`, in the pipeline, answers it with a 400 whose
`details.violations` names `If-Match`, so a client bug cannot silently turn its writes into
last-write-wins.

The real thing is `backend/internal/module/example/internal/`: the rename handler carries
the voter and the explicit 409 path itself, so a write that reaches the bus is checked
where the transaction is.

## What the bus does on `Dispatch`

1. Opens a scoped transaction (`tenancy.md`) and calls the handler with it.
2. Commits on `nil`; rolls back on an error and returns **the handler's own error**.
3. Records an `audit.Entry` either way: action, subject, actor and on-behalf-of (from
   `audit.ActorProvider` - the request's principal), tenant (the scope's, or the command's
   `AuditTenantID()` when the actor is an operator outside any tenant), request id,
   duration, and - on success - the snapshots left on the transaction.

The entry is written **after** the transaction ended, in its own, so a failed command
still leaves its trace. The logger never fails: a trail that could undo a change is a trail
nobody dares to write.

4. Runs what the handler queued with `tx.AfterCommit(fn)`, in order, once the commit has
   happened - never after a rollback. That is where a side effect that must not run inside
   the transaction goes: dropping a cache entry (inside, a concurrent read refills it from
   the old row), sending a mail (its link would point at a row a rollback removes). An
   error there is returned to the caller; the write is already durable.

The handler is wrapped in a deferred rollback, so a panic cannot leave a connection in the
pool with a transaction open.

## A command dispatched from inside a handler

Identity's register creates the workspace through Tenant's own command; Manager's suspend
dispatches Tenant's. The bus recognises the nested `Dispatch` from the context and runs it as
a **savepoint of the running transaction**, on the same connection:

- Both commit or neither does. An inner failure rolls back only the savepoint; the outer
  handler decides whether to carry on.
- The inner command gets its own scope if its context names another tenant, and the outer
  scope is put back when the savepoint is released.
- Its audit entry keeps its own snapshots and is written when the outermost command ends:
  a success only if that commit happened, a failure either way.
- Its `AfterCommit` functions wait for the outermost commit.

A nested dispatch that opened its own transaction would hold a second connection while the
first waits - with every request doing it, the pool runs dry and stays dry. A tenant-setup
step should therefore write through commands, which join the creating transaction, rather
than open its own.

## Snapshots

`tx.Update(old, new)`, `tx.Insert(new)`, `tx.Delete(old)` - the entity implements
`Snapshot() map[string]any` and decides what is worth keeping and what is noise or a
secret (an encrypted field is not in a snapshot). Explicit on purpose: the handler is the
only code that knows. A handler that records nothing produces an entry with no `before` /
`after`, which is honest, and which the Example module's test refuses for its own
commands.

## Optimistic locking

`UPDATE ... SET version = version + 1 WHERE id = $1 AND version = $2`, then
`scope.CheckVersion(tag)`: zero rows is `scope.ErrStale`. The repository re-reads and the
handler answers `httpx.Conflict` with both versions and the current record - the 409 body
the ui-kit's conflict bar reads. Checked where the row is, never with a read-then-write.

## Jobs

A command is synchronous because the caller needs the answer. Work the caller should not
wait on is a job: the handler enqueues it with `svc.Outbox.Enqueue(ctx, tx, job)` **in the
same transaction** (`events-and-outbox.md`), and the worker registered with
`outbox.Handle(svc.Outbox, fn)` runs it after the commit under the same tenant. A job that
writes dispatches a command of its own, so the job's write is in the trail like any other
(`example/internal/job.go`).

## Where it is enforced

- `writeboundary`: `*scope.Tx` only in `handler.go`, `repository.go`, `job.go`;
  `scope.Pool` only in `repository.go`.
- `rawsql`: SQL only in `repository.go`.
- `thincontroller`: `api.go` cannot reach a handler or a job; it dispatches, and it may read through the repository.
- `kernel/command/bus_test.go`: commit, rollback, the failed entry, the operator's tenant,
  typed dispatch, double registration - against Postgres.
