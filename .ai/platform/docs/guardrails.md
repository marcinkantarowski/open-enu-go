# Guardrails

Every rule that matters is a check that fails. This is the list, what each one catches,
where its proof lives, and how it is proved to fire. A rule absent from this page is a
rule that is not enforced; a convention added without its row and its check is gone in
three months.

## Enforced by the compiler

| Rule | How |
|---|---|
| A module imports nothing from another module but its `contract/` | Go `internal/` packages: `x/internal/...` is importable only from under `x/` |
| The kernel knows nothing about the application | `github.com/open-enu/kernel` is a separate Go module with no dependency on `github.com/open-enu/app`; `make selftest` plants the import and watches `go build` fail |
| A handler that disagrees with the spec does not exist | `api.go` implements the generated `StrictServerInterface`; a missing or mistyped method is a build error |
| Only a handler writes | `*scope.Tx` is handed to handlers by the bus and to nothing else; `writeboundary` adds the file rule |

## Enforced by `kernel/analysis` - `make analyzers`, in `make check`

Each rule is one `go/analysis` package with a `testdata/` fixture that `go test` proves
fails. `kernel/analysis/cmd/arch` runs them all over `./...` of both modules.

| Analyzer | Catches | Why |
|---|---|---|
| `thincontroller` | `api.go` importing anything but stdlib, `kernel/httpx`, `kernel/i18n`, `kernel/command`, `kernel/platform`, `kernel/flags`, `kernel/auth` and its own `contract/`; `api.go` using an identifier declared in `handler.go` or `job.go` | a controller turns a request into a command and may read through the repository; it never calls a handler or runs a job. `repository.go` is not on the forbidden list: `writeboundary` and `rawsql` already keep the transaction and the SQL out of `api.go` |
| `rawsql` | pgx `Exec`/`Query`/`QueryRow`/`CopyFrom`/`SendBatch` outside `repository.go` or a `_test.go`; `scope.Unscoped` outside those, or anywhere with a non-literal or empty reason | SQL in one file per module is SQL a schema change can find; a literal reason is what makes `grep Unscoped` a complete review of tenant-isolation lifts |
| `platformservice` | `os.WriteFile/Create/OpenFile/Mkdir*/Remove*/Rename` and a raw Redis client import outside `kernel/` | files and cache go through the kernel's contracts, which scope them to the tenant and include them in erasure and backup |
| `rawstring` | a string constant used as an `i18n.Key` that is not dotted lowercase - in a call, a conversion or a declaration | every user-facing string is a catalogue key; the type stops variables, this stops `NotFound("Project not found.")` |
| `flagdeclared` | a flag key read through `flags.Flags` that is not a string literal, or has no `flags.Definition` literal in the same package | an undeclared flag resolves to the call's default forever - invisible to an operator, impossible to switch off during an incident |
| `impersonationguard` | in a module's `api.go`, a method of `StrictServerInterface` that calls `command.Dispatch` or `bus.Dispatch` with no `auth.Guard.DenyImpersonated` before it, unless the method carries `//impersonation:allowed <reason>`; that comment with no reason | support may look, support may not act in someone else's name ([ADR-0020](../adr/0020-impersonation.md)): the guard is the default, and a route without it - a public `/api/auth/*` route has no principal to impersonate - says why where the next reader will see it |
| `textlen` | in a module's `api.go` or `handler.go`, `len()` of a string compared with a non-zero constant | `len` counts bytes; the spec's `maxLength` and `VARCHAR(n)` count characters, so a byte count refuses a Polish name both of them accept. `utf8.RuneCountInString`, or `len([]byte(s))` for a limit that really is in octets |
| `tableowner` | in a module, a SQL string literal naming a table another module's migrations create; a migration `REFERENCES` one | a module's tables are its own: others reach the data through its `contract/`, so the owner can change its schema and erasure stays where it put it. All SQL is raw, so this is what holds the line |
| `writeboundary` | in a module, a value of type `*scope.Tx` in any file but `handler.go`, `repository.go`, `job.go`; a `scope.Pool` in any file but `repository.go` | only the bus opens a write transaction and only a handler receives it - that is what makes audit coverage structural |

**Not enforced:** a `WHERE` on an encrypted column. A SQL string does not say which column
holds ciphertext, so no analyzer here can see it; the rule is held by review
([`review-change`](../skills/review-change.md)) and by the `*_hash` sibling column that
`svc.Crypto.HashForLookup` fills for equality lookups.

## Enforced by Postgres

Row-level security on every scoped table, `FORCE`d, with `WITH CHECK`, under a
`NOSUPERUSER NOBYPASSRLS` role ([`tenancy.md`](tenancy.md)). Proved in
`kernel/scope/tx_test.go` against a real server; `make ci` runs it, `make check` skips it
visibly (`testing.md`).

## Enforced by the registry - at start-up and in `openenu modules`

`module.NewRegistry` refuses, naming the fix: a permission or a flag without the owner's
prefix or declared twice, a flag identifier that is not dotted lowercase, and a catalogue
key present in `messages.en.json` and absent from `messages.pl.json` (or the reverse).
`make selftest` plants a locale parity break in the scaffolded module and expects
`openenu modules` to fail.

## Enforced by `go generate` + diff - `make gen-check`, in `make check`

| File | Generator | Stale when |
|---|---|---|
| `cmd/openenu/module_registry_gen.go` | `tools/genregistry` | a module directory was added or removed |
| `backend/openapi.json` | `tools/genspec` | any fragment or the shared components changed |
| `internal/module/<x>/internal/api.gen.go` | `tools/genapi` | that module's fragment changed |

## Enforced by shell and by `openenu`

The first five run in `make check`; all of them but route coverage run in `make arch`;
route coverage runs in `make ci`, after `make test` wrote its trace.

| Check | Catches |
|---|---|
| `check-docs.sh` (`make docs-check`) | a relative link to nothing; a record not in its index; a project record in the platform's tree; an export from `contract/` not named in `MODULE.md` |
| `agents-budget.sh` (`make agents-budget`) | `AGENTS.md` over 32 KB; a `MODULE.md` over 8 KB; a module over 3000 lines / 40 files; a kernel file over 260 lines or the kernel over 8000 |
| `check-typography.sh` (`make typography-check`) | a long dash anywhere |
| `check-versions.sh` (`make versions-check`) | Postgres, Redis, Mercure, Go or Node named at one version in one file and another elsewhere |
| `check-i18n.sh` (`make i18n-check`) | a key in one locale and not the other: every frontend, manager, ui-kit and landing catalogue (`i18n/locales/{en,pl}.json`, flattened), every backend `messages.{en,pl}.json`, a landing page in `content/en/` without its twin in `content/pl/` (or the reverse), and a key a module can put in `details.violations` with no text in an app, which a page would show raw |
| `check-prod-compose.sh` (`make prod-check`) | in the rendered production stack: a source bind mount, a published database or cache port, an unpinned image, a router with no certificate resolver, a dev service, air, a backend container not built from the `prod` target; in staging: a published port, a router outside the allowlist; a script naming `compose.dev.yml` by hand |
| `openenu inventory -check` (`make inventory-check`) | a committed `.ai/inventory.json` that no longer matches what the modules declare |
| `tools/genacl -check` (in `make gen-check`) | a changed `auth.Require` literal with a stale `cmd/openenu/acl_gen.go` - the MCP catalogue would offer the wrong permission |
| `check-dead-code.sh` (`make dead-code`) | unreachable Go under `internal/` or `cmd/` (deadcode, blocking); unused TypeScript (knip, blocking); unreached kernel exports are printed |
| `check-shell.sh` (`make shell-check`) | `sed -i` under `scripts/dev` or `scripts/lib` (BSD sed reads it differently); anything ShellCheck reports at `-S warning`, run through Docker and distinguishing "clean" from "could not run" |
| `openenu route-coverage` (`make route-coverage`) | an operation in a fragment that no test reached while `make test` ran with `ROUTE_TRACE`; a missing trace |

## Proved to fire - `make selftest`

A throwaway copy of the tree, broken on purpose, one case per rule: the kernel importing
the app; `init` touching the kernel; a long dash; a broken link and an unindexed record;
a scaffolded module that must build, serve and test; a stale registry, merged spec and
generated server; one planted violation per analyzer in a real module. The copy's own
scripts are run, never the real tree's (`lessons/a-selftest-runs-the-copys-scripts.md`).

## Adding a rule

In one change: the analyzer (or shell check), its `testdata/` fixture with `// want`
comments, the plant in `selftest.sh`, the row above, and the row in `AGENTS.md`'s Never
list if an agent would otherwise do the thing. A rule without all five is a hint, and
hints do not survive.
