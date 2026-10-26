# Audit - `audit`

Reads the trail. The kernel writes it (`kernel/audit`, every command, both snapshots, the
actor and whoever they acted for), so this module owns no table and one write: GDPR
erasure anonymises a person's entries rather than deleting them - a trail with holes is
not a trail.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `GET /api/audit?limit=` → `{items}`, newest first, 50 by default,
  200 at most; always the caller's own workspace
- Permissions: `audit.view`
- Commands: `audit.actor.anonymised` (the erasure itself, in the trail)
- Tables: none of its own (reads the kernel's `audit_entry`)
- GDPR: exports a person's actions; erasure sets the actor to `erased` and drops snapshots

## Public contract

`contract.Entry`; `contract.Reader` (`Recent(ctx, tenantID, limit)` - a tenant id reads under
that scope, `nil` reads across every tenant and is for the operator realm only) built by
`contract.NewReader`; `contract.MaxLimit` and `contract.DefaultLimit`.

## Notes for agents

- The tenant route never takes a tenant parameter. Cross-tenant reading is Manager's
  (`GET /api/manager/audit`), through `contract.Reader` with `nil`.
- Snapshots are stored as the handler gave them. A module that must not have a field in the
  trail leaves it out of `Snapshot()` - Identity does this with the e-mail.

## Depends on

`tenant`.
