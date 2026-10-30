# Settings - `settings`

Owns runtime configuration and kill switches: per-tenant overrides of the flags every module
declares in code. There is **no setting table** - `module.Module.Flags` is the catalogue
(`svc.FlagDefinitions`), and a row exists only where one workspace's value diverges from the
declaration. It implements the kernel's `flags.Store`, so every `svc.Flags.Enabled` call
written anywhere reads live values with no call site changing.

> Budget: 8 KB.

## Surface

- API (`openapi.json`): `GET /api/settings` (every declaration with `effectiveValue` and
  `overridden`), `PUT /api/settings/{identifier}` (only `tenantEditable` ones; 400 otherwise),
  `GET /api/manager/settings`, `PUT /api/manager/tenants/{tenantId}/settings/{identifier}`
  (operators override anything - the kill switch)
- Permissions: `settings.view`, `settings.manage`; the tenant PUT is refused under impersonation (ADR-0020)
- Commands: `settings.flag.set` (before: the previous value; after: the new row)
- Tables: `setting_override` (RLS; `value` boxed as `{"value": ...}`)
- Kernel hook: `Module.FlagStore` - the resolver the process builds reads this table

## Public contract

Nothing under `contract/`. The kernel's `flags.Flags` is the contract; this module implements
its store.

## Notes for agents

- A write invalidates the tenant's cache tag and the module tag **after the commit**, so a
  switch bites on the very next request. Reads are cached a minute as a backstop.
- A disabled flag makes its route **404, not 403** (Example's archive): "not built yet" and
  "switched off after an incident" are indistinguishable from outside.
- `{"value": null}` is a value. Absence of the key is read the same way; the decoder cannot
  tell them apart.

## Depends on

`tenant` (an operator's override names a workspace that must exist).
