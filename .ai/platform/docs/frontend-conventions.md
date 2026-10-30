# Frontend conventions

Three apps, one shared layer, and one Nuxt layer per feature.

```
ui-kit/          @open-enu/ui-kit - the layer all three extend. Never renamed by `make init`.
frontend/        app.${DOMAIN}     - the tenant application. SPA.
manager/         manager.${DOMAIN} - the operator console. SPA, separate bundle.
landing/         ${DOMAIN}         - marketing. Server-rendered, @nuxt/content.
e2e/             Playwright, against the running stack. `make e2e`, in `make ci` only.
```

## The rules that are not negotiable

**Nothing calls `$fetch` or `fetch` directly.** Every request goes through `useApi()`,
because five behaviours have to be identical everywhere: the in-memory access token, one
shared silent refresh on 401, `credentials: 'include'` so the refresh cookie travels,
409 → a typed `ConflictError`, and 413 → a typed `PayloadTooLargeError` whose `message` is
already the translated sentence with the limit (`error.payloadTooLarge`, from
`details.maxBytes`), so `toast.error(e.message)` reads right. A component that fetches on its own gets none of them, and
the failure is invisible until a token expires. No linter catches it; review does.

**No user-facing string in a component.** Every one is a translation key with an entry in
both `en.json` and `pl.json` ([ADR-0008](../adr/0008-i18n-from-the-start.md)); `make
i18n-check` fails on a key present in one. Nothing lints a template for raw text, so that
half is review too ([`i18n.md`](i18n.md)).

**API types are generated, never written.** `ui-kit/types/api.d.ts` comes from
`backend/openapi.json` via `make types`, and `make gen-check` (in `make arch`) fails on a
stale copy ([`api-contract.md`](api-contract.md)).

**A layer never sets a path.** `srcDir`, `dir.*` and friends in a module's `nuxt.config.ts`
become the *app's* - see
[the lesson](../lessons/a-layers-config-is-the-apps-config.md).

## Adding a feature

`make module NAME=billing` writes both halves. The frontend one:

```
frontend/app/modules/billing/
├── nuxt.config.ts       the layer manifest - i18n only, never paths
├── navigation.ts        defineNavigation([...]) - menu entries, permission-filtered
├── settings-tabs.ts     defineSettingsTab([...]) - optional, a tab on the Settings screen
├── injections.ts        defineInjection(...) - optional, to extend someone else's page
├── pages/billing/       auto-registered; the directory IS the URL
├── components/ composables/ stores/ types/
└── i18n/locales/{en,pl}.{json,ts}
```

Nothing central is edited. `frontend/nuxt.config.ts` lists `app/modules/*/` with a
`nuxt.config.ts` as layers, `useNavigation` globs the `navigation.ts` and `settings-tabs.ts`
files, a client plugin globs `injections.ts`, and deleting the folder removes the feature
with no dangling imports ([ADR-0007](../adr/0007-frontend-modules-as-layers.md)). Nuxt
reads its list of layers once, at startup: `make module` restarts the frontend container
for that reason.

The `.ts` beside each `.json` catalogue is a two-line re-export and is not optional: pointing
the i18n loader at a `.json` puts Vite's json plugin around the module's own transform and
breaks server rendering. The `.json` stays because the parity check reads it as data.

### Other ways to sign in

`login.vue` checks an email and a password, and below the form renders
`<InjectionPoint name="auth.login.methods">`. A module that signs people in another way
registers a link to its own page there, and its endpoint ends in Identity's
`contract.Minter.StartSession`, so the browser gets a login's answer and keeps the session
with `useAuthStore().setSession()` ([`auth.md`](auth.md) → *Other ways to sign in*).

### The dashboard, and the reference module

The dashboard (`frontend/app/pages/index.vue`) has no content of its own: a greeting and
`<InjectionPoint name="dashboard.widgets">` in a two-column grid. What belongs on a product's
first screen belongs to its modules, so a module puts a card there from its `injections.ts`:

```ts
defineInjection('dashboard.widgets', BalanceWidget, { id: 'wallet.balance', order: 10 })
```

`order` decides the position (default 100); a card that wants the full width adds
`lg:col-span-2` to its own root. A widget fetches its own data and must render nothing for
someone without the permission to see it. An `<InjectionPoint>` with a `context` binds it
to every widget as props.

`Example` is the platform's reference module and is not a product screen. Its menu entry is
marked `demo: true` and is offered only while `DEMO_CONTENT` is on (`.env`; off by default,
and always off in production). The module, its routes and `/example` stay either way, because the
platform's browser and security tests stand on them - never delete it to tidy a menu.

### Screens for an operator

Most features have nothing for an operator to do. When one does, `make manager-module
NAME=billing` gives the existing module a layer under `manager/app/modules/<name>/`,
discovered by `manager/nuxt.config.ts` the same way; its endpoints are `/api/manager/...`
behind `auth.RequireOperator`, and its usual way in is a card on the tenant page through
`<InjectionPoint name="manager.tenant">`, which hands each card `tenantId`. The layout and
the rules: [`module-development.md`](module-development.md) → *Screens for an operator*.

**The screen to copy is `frontend/app/modules/example/pages/example/index.vue`.** It is the only
place where optimistic locking, the conflict bar, a realtime refresh, an upload and a
flag-guarded action are all shown working against the real API at once.

## What the shared layer gives you

| Need | Reach for |
|---|---|
| Any HTTP call | `useApi()` - `get/post/put/patch/del`, `ifMatch`, typed `ApiError` / `ConflictError` / `PayloadTooLargeError` |
| Login, logout, restore, switch tenant | `useAuth()` (`useOperator()` in `manager/`) |
| Who is signed in, what they may do | `useAuthStore()` - `can()`, `role`, `tenantName`, `setSession()` |
| A 409 | `useConflict()` + `<ConflictBar>` - reload / overwrite / dismiss |
| Live updates | `useAppEvent('example.project.created', fn)` - one stream per tab ([`dev-stack.md`](dev-stack.md) → *Realtime*) |
| A long job | `useProgress(id)`, or `<JobProgress :job-id>` from the Progress module's layer |
| Uploading a file | `useUpload()` or `<UploadField :owner>` |
| Telling the user something | `useToast()` - errors stay until dismissed |
| A feature flag | `useFlags()` - advisory; the server still 404s a disabled route |
| A menu entry, a settings tab, a widget | `defineNavigation`, `defineSettingsTab`, `defineInjection` + `<InjectionPoint>` |
| Buttons, fields, tables, modals | the `Ui*` components in `ui-kit/app/components/ui/` |

## Permissions

`auth.can('example.manage')` decides what to **render**. It never decides what is allowed -
the server checks every request against the same rules, from Identity's role rule
(`backend/internal/module/identity/internal/permissions.go`), which also computes the list
the session carries. Hide a control the session cannot use; never rely on hiding it.

## Why two SPAs and one server-rendered site

The access token lives in memory and the refresh cookie is `HttpOnly` and scoped to the
API's `/api/auth` ([ADR-0018](../adr/0018-token-transport.md)), so a server render of
`frontend` or `manager` has no session to render *with* - it would emit a signed-out shell
that flips on hydration. Neither is a page anyone needs indexed. `landing` is the opposite
on every count, so it renders on the server ([`landing.md`](landing.md)).

`manager` is a separate bundle on its own host so that the operator's screens are never one
router mistake away from a tenant's browser; the two realms share a signing key and are
kept apart by the token's audience ([ADR-0019](../adr/0019-realm-isolation-by-audience.md)).
An operator session has no refresh cookie, so a reload signs the operator out - by design.

## Before proposing a change

```bash
make typecheck   # all three apps, with vue-tsc, in the node container
make i18n-check  # every catalogue has the same keys in en and pl
make dead-code   # unused TypeScript (knip) fails the build
make e2e         # the browser suite - needs the stack up
```
