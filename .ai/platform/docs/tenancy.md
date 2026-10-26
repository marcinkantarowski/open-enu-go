# Tenancy

Tenant isolation is Postgres's job, not the query author's ([ADR-0009](../adr/0009-tenancy-by-row-level-security.md)).
Every tenant-scoped table carries a row-level security policy; every transaction the
kernel opens tells Postgres which tenant it acts for; a query that forgets the predicate
still returns only that tenant's rows - and a transaction with no tenant returns none.

## How a request becomes a scope

1. The auth middleware puts the principal's tenant on the context:
   `scope.WithTenant(ctx, id)`. A worker does the same from the job's `tenantId`; a test
   calls it directly.
2. `scope.Pool.Begin` (the bus, for writes) or `scope.Pool.Run` (a repository, for reads)
   opens a transaction and runs `SELECT set_config('app.tenant_id', $1, true)` - a
   `SET LOCAL`, gone at commit or rollback. **Never session-level**: the connection goes
   back to the pool and the next borrower must not inherit a tenant. `scope_test.go` proves
   two sequential transactions on one connection see only their own rows.
3. The policy on every scoped table:

```sql
CREATE POLICY tenant_isolation ON example_project
  USING      (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
              OR current_setting('app.bypass', true) = 'on')
  WITH CHECK (same expression);
ALTER TABLE example_project ENABLE ROW LEVEL SECURITY;
ALTER TABLE example_project FORCE ROW LEVEL SECURITY;
```

`scope.Policy(table, column, kind)` writes exactly this; a module's migration ends with
it. `WITH CHECK` means an INSERT or UPDATE whose `tenant_id` disagrees with the scope is
refused, not silently stored where nobody can read it. `FORCE` means the table's owner is
not exempt either.

## Fail closed

No tenant in scope: `current_setting` is empty, `NULLIF` makes it `NULL`, the comparison
is `NULL`, no row matches. The database enforces it, so it holds for every query on a
scoped table - hand-written SQL included - whether or not its author remembered the tenant.

## The application role is not a superuser

A superuser bypasses RLS no matter what the policy says. The api and the worker connect as
a role created `NOSUPERUSER NOBYPASSRLS` (`testdb.AppRole` in tests, the stack's `DB_USER`
in Docker); migrations run as the owner through `DATABASE_ADMIN_URL`. A deployment that
hands the application the owner's DSN has no tenant isolation, and nothing in the code
would tell you. The stack's role is created that way by `docker/postgres/init/01-init.sh`;
nothing checks the attributes of a role configured by hand.

## Reading across tenants

```go
scope.Unscoped(ctx, "operator audit view: every tenant's entries", func(ctx context.Context) error {
	return pool.Run(ctx, func(ctx context.Context, tx *scope.Tx) error { ... })
})
```

Sets `app.bypass = 'on'` instead of a tenant. The reason is a **string literal** and the
call lives in a `repository.go` - the `rawsql` analyzer refuses anything else - so that
`grep Unscoped` is a complete review of every place isolation is lifted. Inside, the
ambient tenant is gone: a write must name its tenant explicitly, and the policy's `WITH
CHECK` still applies to what it names.

## What a module does

- Every table with a `tenant_id` ends its migration with `scope.Policy`.
- Reads go through `svc.DB.Run` in `repository.go`; writes through a command the bus runs.
- Nothing else. No `WHERE tenant_id = ?` by hand - it is harmless but it hides that the
  policy is what protects you, and the day it is missing the predicate will be too.

## Where it is proved

`kernel/scope/tx_test.go`: fail-closed, A sees A, B sees B, bypass sees all, no leak across
a pooled connection, `WITH CHECK` on a smuggled insert, an UPDATE on an invisible row
touches nothing. `kernel/audit`, `kernel/search` and `kernel/outbox` repeat the tenant half
on their own tables. All against a real Postgres (`testing.md`).
