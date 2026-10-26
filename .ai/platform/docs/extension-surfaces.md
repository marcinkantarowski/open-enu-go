# Extension surfaces

"Modify nothing, extend everything" is a slogan until the hooks are listed. These are ours.

Each row says **where to hook in** and **what it costs**. Anything not listed is not an
extension point - if you find yourself editing a file that several features share, stop:
that is the coupling these surfaces exist to avoid. Every slot below is a field on
`module.Module` (`backend/kernel/module/module.go`); why there is no general decorator is
[ADR-0005](../adr/0005-extension-by-named-slots.md).

---

## Adding behaviour

| To… | Hook | How |
|---|---|---|
| **Add a feature** | a module | `make module NAME=billing` - backend module, spec fragment and server, frontend layer, docs, spec stub and Task Router row in one command |
| **Add routes** | `openapi.json` fragment + `Mount` | Write the operation in the module's fragment, `make generate`, implement the strict-server method in `internal/api.go`. `Mount` is skipped for a disabled module, so its routes are gone with it |
| **Add a permission** | `Permissions` | `<module>.<action>`. The registry refuses a string without the owner's prefix and a string two modules both claim |
| **Add a database table** | `migrations/NNNN_*.sql` | goose SQL in *your* module's `migrations/`, embedded by `module.go`. Only your own tables: `tableowner` refuses a query or a `REFERENCES` naming another module's |
| **Add translations** | `i18n/messages.{en,pl}.json` | The registry refuses a key present in one locale and absent from the other ([i18n.md](i18n.md)) |
| **Add a feature flag** | `Flags` | A `flags.Definition` in the package that reads it (`flagdeclared` refuses an undeclared read), listed in `Module.Flags` so an operator can switch it. **Without a declaration the flag can never be turned off**, which is the one job a kill switch has |
| **React to something happening** | `svc.Outbox.Subscribe(name, fn)` in `Mount` | Runs in a worker after the event's transaction committed, retried on error. Many subscribers may react to one event |
| **Do work in the background** | `svc.Outbox.Enqueue(ctx, tx, job)` from a handler | The job goes out only if the handler's transaction commits; it runs under the tenant and request id it was enqueued with. Ids only, never rows (`internal/job.go`) |
| **Do work on a clock** | `Tasks` | `module.Task{Name, Cadence, Run}` with `EveryMinute` or `Daily`; `openenu scheduler` runs it. Idempotent and safe to miss - work that must not be missed is a job (`task.go`) |
| **Put things in place for a new tenant** | `Setup` | A `setup.TenantSetup`; runs on tenant creation, and its seed on `openenu user create --seed`, in dependency order (`setup.go`) |
| **Answer GDPR export and erasure** | `Gdpr` | A `gdpr.Subject` for what the module holds about a person |
| **Make records findable** | `Search` | Declare the entity type **and** the permission that gates it; index from a handler (`search.go`). Never an encrypted column |
| **Tell a person something happened** | `Notifications` | Declare the type; raise it with `svc.Notifier` (`notification.go`) |
| **Add fields to every log line** | `LogFields` | A `logctx.Field` reading the context only; added after the kernel's request, tenant, caller and command ([observability.md](observability.md)) |
| **Influence which language a response speaks** | `Locale` | An `httpx.LocalePreference`, consulted before `Accept-Language`, a module's before those of the modules it depends on |
| **Add a console verb** | `backend/cmd/openenu/console.go` | The one central file a module touches: the verb calls an exported function of the module's root package (`attachment.SweepOrphans`). Work that runs on a clock needs no verb - it is a `Task` |

### Slots with one provider

These take one module's implementation of a kernel interface; the registry refuses a second.
Without a provider the kernel's default stands.

| Slot | Interface | Provided by | Default without one |
|---|---|---|---|
| `Keys` | `auth.KeyResolver` | ApiKey | no API key authenticates |
| `FlagStore` | `flags.Store` | Settings | every flag resolves to its declared default |
| `Notifier` | `notification.Notifier` | Notification | a log line |

**A new slot** is a field on `module.Module`, a `Registry` method that collects it (or picks
the one provider and refuses two), a test in `hooks_test.go`, and a row here - copy `FlagStore`.
It is a change to the kernel's public contract: Ask First.

**Replacing a platform service** (`Mail`, `Storage`, the cache backend) is not a module's
business. The process builds `platform.Services` in `backend/cmd/openenu/platform.go`; wrap
the interface there. That is the decorator, in plain Go, in the one place that composes the
process.

## Reaching another module

This is the part that decides whether the system stays modular. In order of preference:

| Need | Do this | Do **not** |
|---|---|---|
| Read another module's data | Call a reader in its `contract/` package | Import its `internal/` (a compile error) or query its table (`tableowner`) |
| Do something when it changes | Subscribe to its event (`svc.Outbox.Subscribe`) | Call it from yours on every write |
| Make it do something | Dispatch its command through its `contract/` (`tenantc.Provisioner`) - inside your handler it joins your transaction as a savepoint | Write its rows |
| Reference its records | Store the UUID; resolve through its `contract/` | Add a foreign key (`tableowner` refuses the `REFERENCES`) |

Every "do not" fails the build, and each message names the alternative.

**Why the friction is deliberate.** A direct call is always the shortest path, and it is how
a modular monolith becomes a ball of mud: one import, then the table, then a join, and the
module can no longer be reasoned about or moved. The cost of a contract is one exported
function; the cost of skipping it is paid forever.

## Turning things off

| To… | Do this |
|---|---|
| Disable a whole module | `enabled: false` in its `module.yaml`, then `make generate` |
| Disable one route or feature per tenant | a declared flag, read in the handler or `api.go`, switched from the operator console |

A disabled module is not mounted and contributes nothing: no routes, permissions, flags,
tasks, subscribers or setup. Its migrations still exist in the database it already ran on;
`enabled: false` is not an uninstall.

## Frontend

Every hook in the three Nuxt apps is a file in a module layer, found by a glob; nothing
central is edited ([frontend-conventions.md](frontend-conventions.md)).

| To… | Hook |
|---|---|
| Add pages, components, composables | a module layer under `frontend/app/modules/<name>/` - auto-registered |
| Add operator screens | a layer under `manager/app/modules/<name>/` - `make manager-module NAME=X` |
| Add menu entries | `navigation.ts` in the module layer - `defineNavigation([...])` |
| Add a Settings tab | `settings-tabs.ts` in the module layer - `defineSettingsTab([...])` |
| Fill a named region | `defineInjection('<spot>', Component)` in the layer's `injections.ts`. The spots: `auth.login.methods`, `dashboard.widgets`, `example.list.footer`, `manager.tenant` (receives `tenantId`) |
| Override a `ui-kit` component | the same path in your app layer; the original stays importable for wrapping |

---

## What is deliberately *not* extendable

The tenant scope (RLS and `scope.Tx`), the realm check on the token's audience, the bus's
audit of every command, and the outbox. They are not fields on `module.Module` and not
interfaces in `platform.Services`, so there is nothing to put in their place.

A guarantee you can swap out is not a guarantee - adding a slot for one is an Ask First that
needs a very good answer.
