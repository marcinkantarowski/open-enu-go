# Platform services

**Read this before adding any infrastructure to a module.** If a module seems to need
caching, file storage, encryption, feature flags, progress, audit, events, tenant setup or
GDPR, the kernel already owns that contract. Re-implementing one locally is the single most
expensive mistake available here, and the analyzers refuse the obvious forms of it.

A module receives everything in one value: `Mount(mux, svc *platform.Services)`. It keeps
the fields it needs and hands them to its handlers. A test gets the same value from
`platform.InMemory(nil)` - every service real, none of them backed by a network - so a
module's rules are tested against the real constraints, never a mock.

| Field | Contract | Default in `InMemory` | In the process (`openenu api`) |
|---|---|---|---|
| `Flags` | `flags.Flags` | `flags.Static{}` | `flags.Resolver` over every module's `Flags`, with per-tenant overrides from the Settings module's `setting_override` (`Module.FlagStore`), cached a minute and invalidated on write |
| `Cache` | `*cache.Tenant` | memory | Redis when `REDIS_ADDR` is set, else memory |
| `Storage` | `storage.Storage` | memory | `storage.Local` under `STORAGE_ROOT`, in dev and in prod; no S3 implementation exists - it would be a second `storage.Storage`, not a change to callers |
| `Progress` | `progress.Reporter` | recorder | `progress.Postgres` - persists to `progress_job` and pushes to `/tenants/{tid}/progress/{id}`; the Progress module reads it back |
| `Events` | `*event.Broadcaster` | recorder hub | `realtime.Hub` (Mercure) when `MERCURE_URL` is set, else a log hub |
| `Audit` | `audit.Logger` | recorder | `audit.Postgres` into `audit_entry`; the Audit module reads it |
| `Actor` | `audit.ActorProvider` | nobody | `auth.Actor` - the request's principal (a person, an API key, an impersonator) |
| `Crypto` | `*crypto.Encryptor` | test key | `APP_ENCRYPTION_KEY` |
| `I18n` | `*i18n.Catalogue` | kernel keys, en + pl | kernel + every module's `i18n/` |
| `Log` | `*slog.Logger` | default | JSON to stdout; every line logged with a context carries `request_id`, `tenant_id`, the caller and the command ([`observability.md`](observability.md)) |

| `DB` | `*scope.Pool` | - (`platform.WithDatabase`) | `DATABASE_URL`, the application role |
| `Bus` | `*command.Bus` | - | the one way a write happens ([`commands-and-audit.md`](commands-and-audit.md)) |
| `Outbox` | `*outbox.Outbox` | - | River; `openenu worker` works it ([`events-and-outbox.md`](events-and-outbox.md)) |
| `Search` | `search.Indexer` | - | tsvector on `search_index` |
| `Notifier` | `notification.Notifier` | recorder | `notification.Log` until the Notification module provides its own (`Module.Notifier`): a row per recipient, mail when the type asks |
| `Searchable`, `NotificationDefinitions`, `FlagDefinitions`, `Permissions` | slices | empty | the registry's: what every module declared, for the module that answers for all of them |

The database half is attached by `platform.WithDatabase(svc, pool, opts, persistAudit)`;
`InMemory` alone has none, and `Validate` says so. A module test that needs a table adds
`testdb.Postgres(t)` ([`testing.md`](testing.md)).

---

## Feature flags - `kernel/flags`

Declare beside the code the flag guards, in `internal/flags.go`, and list it on the module:

```go
var Flags = []flags.Definition{
	{Identifier: "example.archive", Name: "Archive projects", Description: "...", Default: false,
		TenantEditable: false, Category: "example"},
}
// module.go:  Flags: internal.Flags,
```

Read it through `svc.Flags`:

```go
if !a.flags.Enabled(ctx, "example.archive", false) {
	return nil, httpx.NotFound(httpx.KeyNotFound) // 404, not 403: a kill switch leaks nothing
}
```

Resolution is the declaration's default, then the tenant's override from the context's
tenant. The key is a string literal, prefixed with the module name, and the `flagdeclared`
analyzer refuses a read of a key with no `Definition` in the same package: an undeclared
flag resolves to the call's default forever, invisible to an operator, and cannot be turned
off during an incident - which is the one job a kill switch has.

## Cache - `kernel/cache`

```go
totals, err := cache.GetJSON(ctx, svc.Cache, "example", "totals", 5*time.Minute,
	func(ctx context.Context) (Totals, error) { return repo.Totals(ctx) })
_ = svc.Cache.InvalidateModule(ctx, "example")
```

Every key is prefixed with the tenant from the context, so a value computed for tenant A
cannot be served to tenant B even under the same name; every entry is tagged
`module.<name>` and `tenant.<id>`, so dropping a module's or a tenant's entries is one
call. A raw Redis client in a module is refused by `platformservice`.

## Storage - `kernel/storage`

```go
key, err := svc.Storage.Write(ctx, "attachment", upload.Filename, r, contentType)
// persist key on the owning row; later:
rc, err := svc.Storage.Open(ctx, key)          // ErrNotFound under another tenant's context
url, err := svc.Storage.TemporaryURL(ctx, key, 5*time.Minute)
```

The key is `<tenant>/<module>/<id>-<name>`; a read under a different tenant is a not-found,
not a permission error, and so is a malformed key. Temporary URLs are signed over the key
and the expiry. `os.WriteFile` and friends outside the kernel are refused.

## Field encryption - `kernel/crypto`

Format `v1:iv:ciphertext:tag` under AES-256-GCM with a key derived per tenant by HKDF from
`APP_ENCRYPTION_KEY`; `crypto_test.go` holds known-answer vectors for the derivation, a
decryption and the lookup hash, so the format cannot drift unnoticed
([ADR-0016](../adr/0016-light-encryption.md)). A module's repository calls `svc.Crypto.Encrypt`
on write and `Decrypt` on read for a column declared encrypted in its `MODULE.md`; there
is no column type that does it implicitly, so the pair is visible in the repository
(`example/internal/repository.go`). What a module also calls is `crypto.HashForLookup(value)` to fill the sibling `*_hash` column that
equality lookups go through. **Never query an encrypted column** - it differs on every
write.

## Progress - `kernel/progress`

```go
id, _ := svc.Progress.Start(ctx, "example.archive", total, "")
for ... { _ = svc.Progress.Advance(ctx, id, 1, name) }
_ = svc.Progress.Finish(ctx, id, map[string]any{"archived": n})   // or Fail(ctx, id, reason)
```

Hand `id` back in the 202 body; the UI's `useProgress` subscribes to it.

## Events - `kernel/event`

A module exports its events from `contract/`: a type with `EventName()` (dotted, past
tense: `example.project.created`), `SubjectID()`, `OccurredAt()`, `Payload()`. One that
also has `RequiresPermission() string` is a **broadcast** and reaches every browser of the
tenant through `svc.Events.Handle(ctx, ev)` on the topic `/tenants/{id}/events`, as
`{event, subjectId, occurredAt, payload, requiresPermission}`. Opt in, never automatic: the
payload crosses a trust boundary. A handler publishes through `svc.Outbox.Publish(ctx, tx,
ev)` - in its transaction - and the worker delivers after the commit
([`events-and-outbox.md`](events-and-outbox.md)).

## Audit - `kernel/audit`

The command bus writes one `audit.Entry` per command, success or failure, with the
`Before` and `After` snapshots the handler left on the transaction (`tx.Update(old, new)`).
A module never records its own entries; the GDPR walker and the bus do. In the process the
logger is `audit.Postgres` (table `audit_entry`, kernel migration 0001, under RLS); in a
test the recorder. `Record` never fails - failing to record a change must not undo the
change. [`commands-and-audit.md`](commands-and-audit.md).

## Search - `kernel/search`

Declare the type in `internal/search.go` and list it on the module - `Search:
internal.Searchable`, a `[]search.Declaration{{EntityType: "example.project", Permission:
"example.view"}}`. The registry refuses a type not prefixed with the module's name or gated
by a permission the module does not declare: an excerpt is content, so a type with no
permission is a type nobody may search. Then `svc.Search.Index(ctx, tx, entityType, id,
[]string{name, description}, tenantID)` from the handler that changed the record, in its
transaction; `Remove` when it stops being findable; `Search(ctx, tx, query, types, limit)`
under the request's scope. One `search_index` table, tsvector, GIN. Never index an
encrypted column: the index stores plaintext. The endpoint is the `search` module's `GET
/api/search`, over `svc.Searchable`: it queries only the types whose permission the caller
holds, before the query runs - an excerpt is content.

## Notifications - `kernel/notification`

Telling a person, or a workspace, that something happened. Declare the type on the module -
`Notifications: []notification.Definition{{Type: "webhook.delivery_failed", TitleKey: "...",
BodyKey: "...", Email: true}}` - with both keys in the module's own catalogue; the registry
refuses a type with another module's prefix, a key the catalogue lacks, or a duplicate. Then
`svc.Notifier.ToUser(ctx, userID, typ, args)` (the tenant from the ambient scope) or
`svc.Notifier.ToTenant(ctx, tenantID, typ, args)` (fans out to every member). `args` are
translation arguments, never prose: the row stores the type and the arguments and the client
renders them in the reader's language. An undeclared type is dropped with a log line. The
default is `notification.Log`; the Notification module stores and mails (`Module.Notifier`,
one provider, like `FlagStore`). Tests use `notification.Recorder` (`InMemory`'s default).

## Tenant setup - `kernel/setup`

Implement `setup.TenantSetup` (`OnTenantCreated`, `SeedExamples` - both idempotent, both
run in dependency order) behind a constructor `func(*platform.Services) setup.TenantSetup`
and set `Setup:` on the module to it - a seed writes rows, so it needs the pool, and the
module value exists before the platform does. `SeedExamples` is separate so demo data never
reaches a customer. `example/internal/setup.go` is the shape.

## GDPR - `kernel/gdpr`

Implement `gdpr.Subject` (`ExportFor`, `EraseFor`) behind a constructor
`func(*platform.Services) gdpr.Subject` and set `Gdpr:` on the module. The walker exports in
dependency order and erases in reverse, and audits both.

## Translation - `kernel/i18n`

Every user-facing string is a key in `i18n/messages.{en,pl}.json` - flat, dotted, namespaced
by module, `%name%` placeholders. The registry refuses a key present in one locale and
absent from the other, and a key two modules both define. An `*httpx.Error` carries a key;
the `Locale` middleware resolves the language (the modules' preferences - Identity's stored
choice, Tenant's default - then `Accept-Language` narrowed to what ships, then the default)
and translates the envelope ([`i18n.md`](i18n.md)).

## Who is asking - `kernel/auth`

`auth.Require(ctx, "example.manage")` in the controller: 401 or 403 or the principal.
`auth.Guard{Audit: svc.Audit}.DenyImpersonated(ctx, action)` before anything support must
not do in someone else's name. The middleware put the principal on the context and entered
its tenant's scope before the module saw the request ([`auth.md`](auth.md)).

## Where the rules are enforced

[`guardrails.md`](guardrails.md): `platformservice` (files, Redis), `flagdeclared` (flags),
`rawstring` (strings), `writeboundary` (`*scope.Tx`, `scope.Pool`), `rawsql` (SQL,
`Unscoped`), the registry (prefixes, locale parity), the compiler (`Mount`'s signature), and
Postgres itself (`tenancy.md`).
