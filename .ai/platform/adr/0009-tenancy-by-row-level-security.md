# ADR-0009 - One database, `tenant_id`, row-level security that fails closed

**Status:** accepted · **Date:** 2026-10-22

## Context
Schema-per-tenant isolates well and makes migrations and provisioning heavy. One shared
schema is cheap - and one forgotten `WHERE tenant_id = ...` away from a leak. With SQL
written by hand there is no central query builder to hang a filter on, and a filter in the
application could not see every statement anyway.

## Decision
One schema, `tenant_id` on every tenant-owned table, and the scoping done by **Postgres
row-level security**:

- Every tenant-owned table ends its migration with `scope.Policy(table, column, kind)`,
  which enables and **forces** RLS and writes one policy, `USING` and `WITH CHECK` alike:

  ```sql
  tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid
  OR current_setting('app.bypass', true) = 'on'
  ```

  (a text column compares the bare setting, so its index stays usable).
- The kernel's `scope` package starts every transaction by setting `app.tenant_id` from the
  request's principal, transaction-local - `set_config(..., true)`, the equivalent of
  `SET LOCAL`, never a session-level `SET`.
- With no setting, both sides of the policy are NULL and **nothing matches**: an
  unauthenticated route, a job with no tenant or a console command that forgot one sees no
  rows rather than all of them.
- The application connects as a role that does not own the tables and is `NOSUPERUSER
  NOBYPASSRLS` (`DATABASE_URL`) - a superuser bypasses RLS whatever the policy says.
  Migrations run as the owner (`DATABASE_ADMIN_URL`, `openenu migrate` only).
  `docker/postgres/init/` creates both roles.
- Reading across tenants is `scope.Unscoped(ctx, "reason", fn)`, which sets `app.bypass`
  in its own transaction. `rawsql` allows it only in a `repository.go` and only with a
  literal reason.

## Considered and rejected
- **Schema per tenant:** every migration runs N times, provisioning creates schemas.
- **A second, bypassing database role** for cross-tenant reads: two pools and two `pg_hba`
  entries for no isolation gain, since the bypass is reachable only through
  `scope.Unscoped` and its analyzer.

## Consequences
- Hand-written SQL is scoped too: the policy sees every statement, and a leak requires
  defeating a default rather than forgetting a predicate.
- `WITH CHECK` refuses a write whose tenant disagrees with the scope instead of storing a
  row nobody can read.
- A pooled connection never carries a setting into the next transaction, because the
  setting is transaction-local.
- Fail-closed shows up as "no data", which is the safe failure and must be diagnosed
  as such ([`docs/tenancy.md`](../docs/tenancy.md)).
- `make test-security` proves the isolation from several directions, including a raw `pgx`
  query with no scope set returning zero rows; `kernel/scope/tx_test.go` proves the policy,
  `WITH CHECK` and that no setting leaks through the pool.
