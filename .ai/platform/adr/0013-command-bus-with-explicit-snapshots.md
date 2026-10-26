# ADR-0013 - Every write is a command; the handler records both states

**Status:** accepted · **Date:** 2026-10-22

## Context
An audit log that records "every mutating command" is aspirational without a single write
path: each controller audits or forgets to, and the forgetting is invisible. And a trail
that says only *that* something changed cannot answer the question an incident asks: what
was it before?

## Decision
Every mutation is a command handled by exactly one handler, dispatched on `command.Bus`.
The bus opens one scoped transaction (ADR-0009), hands the handler a `*scope.Tx`, and writes
one `audit_entry` per command: the action, the actor and whoever they acted for
(impersonation, ADR-0020), the tenant, the subject, the request id, whether it succeeded and
why not, and the before and after state.

The states are recorded explicitly by the handler, on the transaction:
`tx.Update(old, new)`, `tx.Insert(new)`, `tx.Delete(old)`. The entity's `Snapshot()` decides
what is worth keeping - a secret or a large blob stays out. The handler has to read the row
before an update anyway, for the optimistic lock: the repository writes
`UPDATE ... WHERE id = $1 AND version = $2`, and zero rows is a 409 carrying both versions.
The before-state is that same read - no extra query, and exactly what the handler compared
against.

- A command dispatched inside a handler joins the running transaction as a savepoint; its
  audit entry is written when the outermost command ends.
- Side effects that must follow the commit - cache drops, mail - go through
  `tx.AfterCommit(fn)`, so they never outrun a rollback.
- Only a handler receives a `*scope.Tx` (the compiler), and `writeboundary` keeps one out of
  every file but `handler.go`, `repository.go` and `job.go`.

## Considered and rejected
- **A Postgres trigger writing `OLD`/`NEW` as JSONB:** the actor, the request id and the
  command's name live in the application and would have to be smuggled in through
  transaction settings; and a second mechanism beside the handler's would be one too many.
- **Diffing in a generic data layer:** there is no ORM to compute changesets (ADR-0001), and
  a generic diff cannot know which fields are secrets.

## Consequences
- Audit coverage is structural rather than remembered.
- Controllers are thin: validate, dispatch, serialise.
- Handlers read before they write - more code than a framework would hide, and honest code.
- Undo stays possible without being built (ADR-0014).
- A handler that writes a record and then returns an error loses the record with the
  rollback; a failure that must leave a trace is recorded outside the handler's transaction
  ([`lessons/a-handler-cannot-record-its-own-failure.md`](../lessons/a-handler-cannot-record-its-own-failure.md)).
- The shape is in [`docs/commands-and-audit.md`](../docs/commands-and-audit.md); `Example`'s
  `rename` shows the minimum.
