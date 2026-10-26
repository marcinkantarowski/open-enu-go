# ADR-0004 - Module boundaries are enforced by tooling, not discipline

**Status:** accepted · **Date:** 2026-10-22

## Context
"Don't couple modules" holds for three months and then quietly stops holding, especially
when code is written faster than it is reviewed.

## Decision
Every boundary is either a compiler error or an analyzer that fails `make check`:

- **Code.** A module's implementation lives in `internal/`, which Go refuses to let another
  module import. `contract/` is the one importable surface, and `MODULE.md` must name every
  identifier it exports (`make docs-check`).
- **Data.** `tableowner`: no SQL string in a module names a table another module's
  migrations create, and no migration `REFERENCES` one. Store the uuid; reach the record
  through the owner's `contract/`. Kernel tables (`audit_entry`, `search_index`,
  `progress_job`, River's) belong to no module.
- **SQL.** `rawsql`: `pgx` `Exec`/`Query` only in `repository.go`; `scope.Unscoped` only
  there, and only with a literal reason (ADR-0009).
- **Controllers.** `thincontroller`: `api.go` imports only `httpx`, `i18n`, `command`,
  `platform`, `flags` and its own `contract/`, and uses nothing declared in
  `repository.go`, `handler.go` or `job.go`.
- **Writes.** `writeboundary`: a `*scope.Tx` is held only in `handler.go`, `repository.go`
  and `job.go` (ADR-0013).
- **Platform services.** `platformservice`: no `os.WriteFile`, `os.Create` or Redis client
  outside the kernel - files and cache go through the kernel's tenant-scoped contracts.

## Consequences
- A violation is a build failure, not a review comment.
- A legitimate exception is explicit and greppable: `scope.Unscoped(ctx, "reason", fn)`.
- The checks must be fast; `make check` has a 60-second budget.
