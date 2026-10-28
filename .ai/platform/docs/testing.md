# Testing

Two speeds, one rule: a test proves a rule against the real thing that enforces it.

| Command | What runs | Backing services |
|---|---|---|
| `make check` | `go test -short` in both modules | none - container tests skip, visibly |
| `make test`, `make ci` | every test | one Postgres and one Redis, started by testcontainers and shared by every test binary of the run; `make test` runs the two modules with the reaper off and removes the containers itself, because the first session's reaper would remove them under the second |

`make check` is the inner loop and stays under 60 s. `make ci` is the gate: a container
test that finds no Docker **fails** there. A green gate with the isolation proofs skipped
is the worst outcome available, so `testdb` never skips outside `-short`.

## A test that needs the database

```go
func TestRenameConflicts(t *testing.T) {
	db := testdb.Postgres(t)                       // a fresh database on the shared container
	ctx := context.Background()
	if _, err := migrate.Run(ctx, db.Admin, nil, migrate.Source{Name: "example", FS: example.Migrations}); err != nil {
		t.Fatal(err)
	}
	svc := platform.InMemory(nil)                  // every service real, none networked
	if err := platform.WithDatabase(svc, &scope.Pool{Pool: db.App}, outbox.Options{}, false); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	example.Module.Mount(mux, svc)
	// ... httptest against mux, under scope.WithTenant(ctx, tenant)
	trail := svc.Audit.(*audit.Recorder).Entries() // assert on what was recorded
}
```

`db.Admin` is the owner (migrations); `db.App` is the application role - `NOSUPERUSER
NOBYPASSRLS`, so the RLS policies are binding in the test exactly as in production. A test
connected as a superuser would prove nothing about isolation.

## What `InMemory` gives you

Memory cache and storage, a recorder for progress, events and audit, static flags, the
kernel's catalogue, an encryptor under a test key. Every one enforces the real constraints:
the memory cache still prefixes keys by tenant, the memory storage still refuses another
tenant's key. **Never mock one of them** to test a module's rules; swap in the real backing
when the test is about the backing (`cache/redis_test.go` runs the same scenario the
memory test does).

## What goes where

| Test | Lives in | Runs |
|---|---|---|
| a kernel contract against Postgres or Redis | `kernel/<pkg>/*_test.go` | `make test` |
| a module's routes through `httptest` | `internal/module/<x>/<x>_test.go` | `make check` when it needs no table, else `make test` |
| an analyzer's rule | `kernel/analysis/<rule>/testdata/` with `// want` | `make check` |
| isolation and credential handling across modules | `internal/security/*_test.go` | `make test`; `make test-security` alone |
| a guardrail actually wired in | `scripts/dev/selftest.sh` - plants, expects failure | `make ci` |
| the browser's view | `e2e/tests/*.spec.ts` - Playwright | `make e2e` (in `make ci`, never in `make check`) |

## Route coverage

Every request a test makes appends its matched operation id to `backend/.route-trace`
(`kernel/httpx/trace.go`, enabled by the `ROUTE_TRACE` path `make test` sets).
`make route-coverage` subtracts the trace from the spec and fails on any operation no test
reached; it runs in `make ci`. The convention inside each test: one `httptest` request per
route, asserting **content**, never only a status code.

## The browser suite

`e2e/` drives a real browser against the running dev stack - never a mocked API or a
preview build - for what a Go test cannot see: cookie attributes, two realms sharing a
signing key, a connection that stays open. Seventeen tests in seven specs: the cookie login,
its refusals, session restore and sign-out (`login`); switching workspace
(`tenant-switch`); an operator viewing a workspace as a member, and an operator token
refused by the tenant API (`impersonation`); the event stream outliving the page that
opened it (`realtime`); an upload (`upload`); and a project created in one session arriving
in another but not in another tenant, the conflict bar, and an operator-switched flag
starting a job with progress (`example`); and the language - `DEFAULT_LOCALE` until someone
chooses, whatever the browser prefers, then the choice across a reload (`locale`).

One worker, no retries: the tests share one database and one seeded workspace, and a retry
would turn "the SSE connection never arrived" into "passed on the second try". `make e2e`
creates the accounts first (`make e2e-fixtures`); `SPEC=tests/x.spec.ts` narrows it
([`dev-stack.md`](dev-stack.md) → *Browser tests*).

## Timings to expect

The first container test of a run pulls nothing after the first time; starting the shared
containers costs a few seconds, and each package then pays about a second to create its
database. `make ci` with the kernel's container tests is about a minute on a warm host.
