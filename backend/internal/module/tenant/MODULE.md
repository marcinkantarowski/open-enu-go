# Tenant - `tenant`

Owns the workspace itself: its slug, name, status (`pending` → `active` → `suspended`),
plan and default locale. It does not own who belongs to it (Identity), what it has paid
for (no billing module exists), or what is inside it (every other module, by row-level
policy). The table is **not** tenant-scoped - it is the list the scope is drawn from.

> Budget: 8 KB.

## Surface

- API: none. A tenant is created by Identity's register (through `contract.Provisioner`) or
  by `openenu user create` when the slug is new; an operator suspends one through Manager.
  There is no tenant CRUD for tenant users, and no operator route creates one.
- Permissions: `tenant.view`, `tenant.manage`, `tenant.delete` (owner only, Identity's rule)
- Events: `tenant.created` `{tenantId, slug, name}`, `tenant.deleted` `{tenantId, slug}` -
  published **before** the row goes, so a subscriber can still read it
- Tasks: `tenant.purge_unverified` (daily) - pending workspaces older than 7 days are deleted
- Tables: `tenant` (no RLS)
- Console: `openenu tenant purge-unverified [--days 7 --dry-run]`

## Public contract

`contract.Info` (with `Active()`) and the statuses `StatusPending`, `StatusActive`,
`StatusSuspended` (the plan is a plain string, `"free"` at creation; no constant names it);
`contract.Reader` (`Describe`, `BySlug`, `Page`) built by
`contract.NewReader` (`Describe` cached `DescribeTTL` under the tenant's cache tag;
`Columns`/`Scan` for a joining reader); the commands `contract.Create`, `contract.Activate`,
`contract.Suspend`, `contract.Delete` and `contract.Provisioner` that dispatches them; the
events `contract.Created` and `contract.Deleted`.

## Notes for agents

- **Creating a tenant runs every module's `setup.TenantSetup`** inside the tenant's scope,
  in the same command. A module that needs a row per workspace declares a setup step; it
  never subscribes to `tenant.created` for that.
- A slug is a public identifier: `Slugify` + `ValidSlug` are the only way one is made.
- Delete publishes first, removes second. Identity drops memberships on the event.

## Depends on

Nothing.
