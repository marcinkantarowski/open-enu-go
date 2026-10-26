---
tags: [tenancy, kernel, contracts, river]
date: 2026-10-25
---
# Taking context from ambient state breaks the callers that legitimately have none

A tenant read from the context is right for a **read** and wrong for a **write of a row that
already knows its tenant**. Reaching past the row to the ambient scope replaces a fact with a
guess, and then punishes every caller that legitimately has no scope.

## The trap
Indexing a record for search could take the tenant from the scope, like everything else that
is tenant-scoped, and fail closed when there is none:

```go
func (x Indexer) Index(ctx context.Context, tx *scope.Tx, entityType, entityID string, fields []string) error {
	tenant, ok := scope.TenantID(ctx)
	if !ok {
		return errors.New("search: indexing needs a tenant in scope")
	}
	// ...
}
```

Every caller on the request path works. Every caller off it breaks: a job re-indexing after
an import, an operator acting from the manager realm (which has no tenant in scope by design),
a console command, GDPR erasure, and anything inside `scope.Unscoped` - which clears the
request's tenant on purpose, so the deliberate, reasoned escape hatch the tenancy design is
built around ([ADR-0009](../adr/0009-tenancy-by-row-level-security.md)) becomes a hard failure.
And the failure surfaces in whichever suite owns those callers, on a test that has nothing to
do with search.

Failing closed on the ambient scope is right for a read: a query with no tenant must return
nothing rather than everything, because there is no other source of truth about who is asking.
For a write, the row's own tenant id is right there and is authoritative.

## The rule
**If the caller already holds the context, take it as a parameter.** Ambient state is for when
there is genuinely no better source - and then failing closed is right.

## Where it is held
The asymmetry is in the signatures, where it is impossible to miss
(`backend/kernel/search/search.go`):

```go
func (x Indexer) Index(ctx context.Context, tx *scope.Tx, entityType, entityID string, fields []string, tenantID string) error
func (x Indexer) Remove(ctx context.Context, tx *scope.Tx, entityType, entityID, tenantID string) error
func (x Indexer) Search(ctx context.Context, tx *scope.Tx, query string, entityTypes []string, limit int) ([]Hit, error) // the scope's
```

Writes take the tenant, and the RLS policy's `WITH CHECK` refuses a row whose tenant disagrees
with the transaction's scope, so an explicit tenant cannot smuggle a record across. `Search`
reads under the scope: no tenant, no hits.

- `crypto.Encryptor.Encrypt(plaintext, tenantID)` takes the key's owner from the caller, which
  reads it from the row ([[an-encryption-key-must-belong-to-the-row]]).
- `setup.TenantSetup` takes `tenantID` on both of its methods.
- `scope.Unscoped` clears the request's tenant rather than only lifting the policy: a write
  made inside must name its tenant, because code that asked `scope.TenantID` would otherwise
  get the outer tenant back and write it onto rows that belong to another.
- The bus takes an audit entry's tenant from the scope and, when there is none, from the
  command (`command.TenantTargeted`): an operator suspending a tenant has no scope, and an
  entry written without a tenant would vanish from that tenant's trail.

## Where else this applies
- Cache and storage keys take the tenant from the context (`scope.TenantOrGlobal`): right on
  the request path, and silently `global` anywhere without a scope. Called from a job or a
  task, they need the row's tenant put in the context first (`scope.WithTenant`).
- The outbox stores the tenant a job was enqueued under and restores it before the job runs,
  which is why job code reads like request code - and why a job enqueued inside
  `scope.Unscoped` carries no tenant at all.
- The general shape: a service that is fine when called from a controller and fails when
  called from a worker is usually reading ambient state it could have been handed.
