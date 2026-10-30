# kernel - github.com/open-enu/kernel

A separate Go module, so that `go build` itself proves it never depends on the application.
One package per platform concern ([ARCHITECTURE.md](../../.ai/platform/ARCHITECTURE.md) §5); each exports one contract, a default
implementation and an in-memory one a test can reach into, and the `analysis` package
ships the analyzers that force modules to use them.

| Package | Owns |
|---|---|
| `module` | the `Module` declaration (`Manifest`, migrations, locales, OpenAPI fragment, flags, setup, GDPR, tasks, search, key resolver, locale preference, flag store, notifications) and the `Registry` that validates and dependency-orders it; `tools/genregistry` writes the registry from the `module.yaml` files on disk |
| `command` | the write side: `Command`, `Register[C, R]`, `Bus.Dispatch` opening the scoped transaction, recording the audit entry on success and on failure, and running the handler's `AfterCommit` queue once the write is durable |
| `audit` | the trail: `Entry` with actor, on-behalf-of, tenant, request id, before/after; `Slog`, `Recorder` and `Postgres` (`audit_entry`, kernel migration 0001, written synchronously in its own short transaction after the command's); `ActorProvider` |
| `scope` | tenant isolation: the context half (`WithTenant`, `TenantID`), `Pool` and `Tx` carrying `SET LOCAL app.tenant_id` into Postgres for RLS, snapshots for the trail, `AfterCommit(fn)` for side effects the bus runs after the commit, `Unscoped(ctx, reason, fn)`, `CheckVersion`, `Policy`, `VerifyApplicationRole` (refuses a SUPERUSER or BYPASSRLS pool) ([ADR-0009](../../.ai/platform/adr/0009-tenancy-by-row-level-security.md)) |
| `cache` | the only cache a module may use: `Tenant` prefixes every key with the tenant and tags it `tenant.<id>` / `module.<name>`; `Memory` and `Redis` backends |
| `storage` | tenant-scoped files: `<tenant>/<module>/<id>-<name>` keys, `Local` and `Memory` backends, `Signer` and `Download` serving signed, expiring URLs |
| `crypto` | `Encryptor` (AES-256-GCM, HKDF per tenant, pinned to a fixed test vector), `Encryptor.HashForLookup` (HMAC under an HKDF sub-key of the master) for the sibling `*_hash` column, `DeriveKey` for a per-purpose sub-key of a secret, `PasswordHasher` |
| `event` | typed domain events, the `Broadcast` marker, the `Envelope` the browser reads, `Broadcaster` publishing on `/tenants/{tid}/events` |
| `outbox` | events and jobs as River jobs inserted in the caller's transaction (`Publish`, `Enqueue`, `Handle`, `Subscribe`); the worker restores the scope, broadcasts and calls subscribers, with retries and a dead-letter state |
| `realtime` | the Mercure `Hub` the broadcaster publishes to, and the subscriber-token endpoint a browser needs for SSE |
| `search` | Postgres full-text search: `Indexer` over `search_index` (kernel migration 0002) and `Declaration` - an entity type and the permission that gates its excerpts |
| `flags` | `Definition`, the `Flags` contract, `Resolver` (declared default, then the tenant's override from a `Store`), `Static` and `MemoryStore` |
| `progress` | `Reporter` for long jobs (`Start`, `Advance`, `Finish`, `Fail`); `Logging`, `Recorder` and `Postgres` (`progress_job`, kernel migration 0003, publishing each update on `/tenants/{tid}/progress/{id}`) |
| `gdpr` | `Subject` and the `Walker` that exports in dependency order and erases in reverse, auditing both |
| `setup` | `TenantSetup` and the `Runner` that asks every module in dependency order, behind tenant creation and `make seed` |
| `notification` | `Definition`, the `Notifier` contract (`ToUser`, `ToTenant`), `Log` and `Recorder`; the feed itself lives in the Notification module |
| `mail` | `Mailer`: SMTP for the stack, a log line when no `MAILER_DSN` is set, a `Recorder` for tests |
| `auth` | `Principal`, the HS256 `Signer` and its `Claims`, `KeyResolver` for `sk_` keys, `Middleware`, `Require`/`RequireRealm`/`RequireOperator`, the impersonation `Guard`, the audit `Actor` |
| `i18n` | the `Key` type, `Catalogue` loading and translation, `CheckParity` across locales, `Negotiate` for `Accept-Language`; the kernel's own `messages.{en,pl}.json` |
| `httpx` | the error envelope, the 409 `ConflictBody`, `Pagination` + `ListMeta`, `IfMatch`/`ETag` and `ValidIfMatch` (a malformed If-Match is a 400), `MaxBody`/`AllowBody` (the 413 body limit), request id, access log, panic recovery, CORS, rate limit, the locale middleware, health probes, the server bootstrap |
| `platform` | `Services`: the one value a module is handed on mount, `InMemory` for tests, `WithDatabase` for the pool, bus, outbox and indexer |
| `migrate` | goose runs of the kernel's own tables, River's, and every module's `migrations/`, each under its own version table |
| `spec` | the OpenAPI fragment reader the inventory, the MCP catalogue and route coverage share |
| `inventory` | `.ai/inventory.json`: every module and every name it declares, with `-check` for staleness |
| `mcp` | spec + ACL -> tool catalogue, filtered to a key's scopes, served over stdio |
| `testdb` | one Postgres and one Redis for a test run, through testcontainers; every test gets its own database and a non-superuser role so RLS is actually tested |
| `heartbeat` | liveness for the processes that serve no HTTP: `Run`/`Touch` beat `HEARTBEAT_FILE`, `Check` is what `openenu health -heartbeat` reads |
| `analysis` | the guardrail analyzers, one sub-package per rule, each with `testdata/` that breaks it; `analysis/cmd/arch` runs them all |

`job/` is empty apart from a `.gitkeep`: background jobs are the outbox's concern
(`outbox.Enqueue`, `outbox.Handle`), so the directory holds no code.

`kernel.go` carries `Version`, which `make platform-update` compares against.
