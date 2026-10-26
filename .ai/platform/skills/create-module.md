# Skill - create a module

A feature exists in two places that mirror each other: `backend/internal/module/<name>/` and
`frontend/app/modules/<kebab-name>/`. Both are discovered from the directory - the backend by
`tools/genregistry` reading every `module.yaml`, the frontend by Nuxt's layer glob - so there
is no central file to register anything in, and no central file to forget.

---

## 1. Check it does not already exist

```bash
grep -i '<thing>' .ai/inventory.json
make modules
```

A feature that belongs *inside* an existing module goes there. A new module is for a new
noun with its own lifecycle, not for a new verb on an existing one.

## 2. Scaffold it

```bash
make module NAME=invoicing
```

Never by hand. One written by hand differs in ways nobody notices until the third one, and
this generator generates, builds, vets and runs the test of what it writes before it reports
success. It creates:

```
backend/internal/module/invoicing/
  module.go  module.yaml  MODULE.md  openapi.json  invoicing_test.go
  contract/doc.go  i18n/messages.{en,pl}.json  migrations/   (empty until the first table)
  internal/{api.go,api.gen.go,mount.go}
frontend/app/modules/invoicing/
  nuxt.config.ts  navigation.ts  pages/invoicing/index.vue
  i18n/locales/{en,pl}.{json,ts}  components/  composables/  stores/  types/
.ai/specs/<today>-invoicing.md   # .ai/platform/specs/ in the platform's own repository
AGENTS.md  → a Task Router row
```

## 3. Write the spec first

The spec stub is already there, with its row in the index. Fill it in before writing code if
the work has three or more steps or makes an architectural decision. It is cheap and it is
what the next person reads.

## 4. The table and the entity

Start from `backend/internal/module/example/`: `migrations/0001_create_project.sql` and
`internal/entity.go`. Every cross-cutting concern is one line there:

- `tenant_id UUID NOT NULL` plus the `tenant_isolation` policy - row-level security makes
  every query see only the scope's tenant, and nothing with no tenant in scope. Almost always
  right. See [`tenancy.md`](../docs/tenancy.md).
- `version INTEGER` - a concurrent edit becomes a 409 carrying both versions instead of a
  silent overwrite. Right for anything a person edits.
- **No foreign key to another module's table**, and no query naming one. Store the uuid and
  reach the record through that module's `contract/`. `tableowner` refuses both
  ([ADR-0004](../adr/0004-module-boundaries.md)).

A migration is goose SQL in the module's `migrations/`, embedded by `module.go` (the
scaffold's comment there says how). Then `make migrate`.

## 5. Writes go through the bus

A command in `internal/command.go`, a handler in `internal/handler.go`, registered in
`mount.go` with `command.Register(svc.Bus, h.fn)`. `*scope.Tx` outside `handler.go`,
`repository.go` and `job.go` is a build failure (`writeboundary`). See
[`commands-and-audit.md`](../docs/commands-and-audit.md).

## 6. Declare what the module offers

Each is a field on `module.Module` in `module.go` - the full list is
[`extension-surfaces.md`](../docs/extension-surfaces.md):

| Field | Declares |
|---|---|
| `Permissions` | `invoicing.view`, `invoicing.manage` - prefixed with the module, refused otherwise |
| `Search` (`internal/search.go`) | which fields are indexed, and the permission to see them |
| `Flags` (`internal/flags.go`) | feature flags, or they can never be switched |
| `Setup` (`internal/setup.go`) | what a brand-new tenant needs |
| `Gdpr` | what the module holds about a person. Nothing checks this for you - a module with a `user_id` column and no `Gdpr` is silently skipped by export and erasure |

Never index an encrypted column: the index stores plaintext and returns excerpts of it.

## 7. The frontend layer

The directory is the registration. Contribute pages, a `navigation.ts` entry and
translations; **never** set `srcDir` or any other path in a layer's `nuxt.config.ts` - a
layer's config merges into the *app's*, so it moves the app's own pages out from under it
with no error at all ([lesson](../lessons/a-layers-config-is-the-apps-config.md)).

Every request goes through `useApi()`. Never `fetch` or `$fetch` from a component. The
rest - the shared layer, permissions, settings tabs, injection points - is
[`frontend-conventions.md`](../docs/frontend-conventions.md).

## 8. Tests

At least one test per operation - this is measured, not encouraged (`make
route-coverage`). Copy `backend/internal/module/example/project_api_test.go`: a real Postgres
from testcontainers, the module mounted on a real mux, requests through the real middleware.
Assert content, never only a status code; assert the refusals. See
[`testing.md`](../docs/testing.md).

## 9. Finish

```bash
make check        # after every edit
make generate types
make inventory
make ci
```

`MODULE.md` must name everything `contract/` exports - `make docs-check` compares them.

---

## Done when

- [ ] `make ci` is green
- [ ] every operation has a test, and `make route-coverage` says so
- [ ] `MODULE.md` describes what is there, and a Task Router row points at it
- [ ] `.ai/inventory.json` is regenerated
- [ ] no string reaches a user without a key in both locales
