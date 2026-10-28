# Agent Guidelines - open-enu-go

This repository is a foundation for multi-tenant SaaS products - a Go API, three Nuxt apps,
the infrastructure to run them, and the guardrails that keep it in shape - designed to be
**read and extended by AI agents without creating technical debt**. Two properties make
that true, and both are enforced, not hoped:

- **Readable** - paths are *derivable* from a feature name, never discovered by exploration.
- **Extendable without debt** - every rule that matters is also a check that fails.
  Prose is a hint; CI is the contract.

Architecture: [`.ai/platform/ARCHITECTURE.md`](.ai/platform/ARCHITECTURE.md). Every decision
and its rationale, in order: [`.ai/platform/adr/`](.ai/platform/adr/README.md).

> **Instruction budget:** this file stays under **32 KB** - past that, tools truncate it and
> these rules silently stop applying. Hard rules and routing here; long-form procedure in
> `.ai/platform/docs/`. Enforced by `make agents-budget`.

---

## What exists

**The kernel and the module system.** `github.com/open-enu/kernel` (`backend/kernel/`) is a
separate Go module that cannot import the application. A directory under
`backend/internal/module/` with a `module.yaml` is discovered by `go generate` and becomes
routed, migrated, translated, permission-declaring and documented with no central file
edited; `make module NAME=x` scaffolds one across the backend, the frontend, the docs and
this file's Task Router, then builds and tests it.

**The platform contracts.** A command bus that writes every change and its audit entry in
one transaction, with before/after snapshots and a real actor; tenancy by Postgres row-level
security that fails closed; a River outbox for events and jobs; tenant-scoped cache and
storage; field encryption with derived per-tenant keys; optimistic locking with a 409
carrying both versions; full-text search; tenant setup hooks; GDPR export and erasure;
progress; feature flags with per-tenant overrides; notifications; realtime through Mercure.
Read [`platform-services.md`](.ai/platform/docs/platform-services.md) **before** adding any
infrastructure to a module.

**Twelve modules.** Tenant, Identity, ApiKey, Audit, Attachment, Progress, Settings, Manager,
Notification, Webhook, Search and the `Example` reference. A person can register, verify by
mail, log in, belong to several workspaces and switch between them, and reset a password; a
machine authenticates with a scoped API key; operators have their own realm and console, and
impersonate through one audited, non-renewable crossing. Outbound webhooks are signed with
Standard Webhooks and retried; notifications are a feed and mail that store keys, not
sentences.

**`Example` is the module to read and to copy** - one entity, `Project`, with exactly one of
every platform concern. It is capped at 1150 lines including tests (`make agents-budget`)
and sits just under it: a new demonstration there means removing one.

**The guardrails are proved to fail.** Eight `go/analysis` analyzers plus shell checks for
docs, translations, typography, versions, budgets, the production stack and dead code;
`make selftest` breaks a throwaway copy to watch each one fail. `make test-security` proves
tenant and realm isolation from several directions (27 tests).

**The three apps run against the real stack.** `make builddev` brings everything up under
trusted HTTPS and `make smoke` checks 15 URLs for content; `make e2e` drives 17 browser
tests in seven specs - login, session restore, tenant switch, impersonation, realtime,
uploads, the 409 conflict bar, a feature flag switched by an operator and the default language.

**Agent interfaces.** `.ai/inventory.json` indexes every declared name, so "does this exist?"
is a grep; `openenu mcp -key sk_...` offers the API to an MCP client as tools filtered to
what the key may call; every operation is proved to have a test that reaches it.

**Deployment.** `make buildprod` / `make buildstaging` take a bare VPS to a running product
after a read-only preflight, and production refuses to finish without an off-box backup.
These scripts are proved by `selftest` and `prod-check` but **have not yet been run against
a real server**, and `.github/workflows/ci.yml` (the same `make ci`) **has not yet run on
GitHub**. Say so rather than assume either works.

A capability that does not exist is not to be improvised. Say so instead.

## Always

- **Start a module with `make module NAME=x`**, never by hand
  ([`module-development.md`](.ai/platform/docs/module-development.md)).
- **Read [`platform-services.md`](.ai/platform/docs/platform-services.md) before adding any
  infrastructure to a module.** The kernel owns cache, storage, encryption, search,
  progress, flags, audit, events, setup and GDPR, and `Mount` hands them to the module in
  one `*platform.Services`; a module re-implementing one is the most expensive mistake here.
- **The spec comes first.** A route starts in the module's `openapi.json`, then
  `make generate`, then `api.go` implements what the generated interface demands
  ([`api-contract.md`](.ai/platform/docs/api-contract.md)).
- **Every state change is a command and a handler.** Only a handler receives `*scope.Tx`
  ([`commands-and-audit.md`](.ai/platform/docs/commands-and-audit.md)); `tx.Update(old, new)`
  is what puts both states in the trail.
- **Every tenant-scoped table ends its migration with `scope.Policy`.** Isolation is the
  policy's, not the query's ([`tenancy.md`](.ai/platform/docs/tenancy.md)).
- **Events and jobs go through `svc.Outbox.Publish(ctx, tx, ev)` inside the handler's
  transaction** ([`events-and-outbox.md`](.ai/platform/docs/events-and-outbox.md)).
- Check `.ai/platform/adr/` (and a project's own `.ai/adr/`) before changing an
  architectural decision - several deliberate choices look like omissions (no custom
  fields, no hierarchical orgs, no undo).
- Check `.ai/specs/` and `.ai/platform/specs/` for an existing spec before modifying
  anything non-trivial.
- **`.ai/platform/` ships with the platform and is replaced by an update; everything else
  under `.ai/` is the project's.** `"initialized"` in `.project.json` decides where a new ADR,
  spec, lesson or analysis goes ([ADR-0027](.ai/platform/adr/0027-platform-and-project-knowledge-are-separate-trees.md)).
- Write a spec (`{YYYY-MM-DD}-{kebab-title}.md`, on the side `.project.json` names) before
  work with 3+ steps or an architectural decision.
- Keep changes inside one module. Touching several is an `Ask First`.
- Run `make check` after every edit. Budget: 60 s; about 15 s today. It skips the
  container tests; `make test` and `make ci` run them and FAIL without Docker
  ([`testing.md`](.ai/platform/docs/testing.md)).
- When you add a convention, add the analyzer or check that fails when it is broken, **and
  its breaking fixture under `backend/kernel/analysis/<name>/testdata/src/`** (or its proof
  in `scripts/dev/selftest.sh`), in the same change.
- Prefer deleting to deprecating while the repository has no users.

## Ask First

- Reducing scope, changing a public contract (the spec), or changing an ADR.
- Touching more than one module in a single change.
- Adding a dependency to either `go.mod`, or a production npm dependency.
- Anything that mutates a live server, DNS zone or database.
- Renaming, moving or reshaping anything under `backend/kernel/` - it is a versioned module.

## Never

- **Never commit, push, merge or open a pull request.** The user does. Not enforceable from
  the repository: enforce it in the agent's own settings (for Claude Code, a
  `permissions.deny` list in `.claude/settings.json`).
- **Never rename the kernel.** `github.com/open-enu/kernel` and `@open-enu/ui-kit` are the
  framework's identity. `init` renames `github.com/open-enu/app` only
  ([ADR-0002](.ai/platform/adr/0002-kernel-packaging.md)); `make selftest` guards it.
- Never let `backend/kernel/` import `github.com/open-enu/app/...`. The dependency graph
  forbids it; `make selftest` proves the build fails if someone tries.
- Never import another module's `internal/`. Only its `contract/`. The compiler refuses.
- Never run `SET app.tenant_id` - the scope is transaction-local, set by `kernel/scope`
  inside the transaction. A pooled connection with a session-level setting is a cross-tenant
  leak waiting for the next borrower.
- Never read across tenants outside `scope.Unscoped(ctx, "reason", fn)` with a literal
  reason, in a `repository.go`.
- Never query, index or search-index an encrypted column. Add a sibling `*_hash` written
  with `crypto.HashForLookup` and query that.
- Never call `pgx` `Exec`/`Query` outside `repository.go`; never hold a `*scope.Tx` outside
  `handler.go`, `repository.go` or `job.go`; never open `scope.Pool` outside `repository.go`.
  Enforced by `rawsql` and `writeboundary`.
- Never name another module's table in SQL, and never `REFERENCES` one from a migration.
  Store the uuid; reach the record through its `contract/`. Enforced by `tableowner`.
- Never measure user text with `len()` against a limit - it counts bytes, and `maxLength`
  and `VARCHAR(n)` count characters. `utf8.RuneCountInString`; enforced by `textlen`.
- Never say whether an account exists from a public endpoint - not in the answer, not in
  the time it takes ([ADR-0023](.ai/platform/adr/0023-registration-answers-the-same-for-every-address.md)).
- Never hand the application the database owner's DSN. A superuser bypasses every RLS
  policy and nothing in the code would tell you. `DATABASE_URL` is the `NOSUPERUSER
  NOBYPASSRLS` role; `DATABASE_ADMIN_URL` is for `openenu migrate` only.
- Never create a tenant-scoped table without `scope.Policy` at the end of its migration.
- Never use `os.WriteFile`, `os.Create` or a raw Redis client outside `kernel/`.
- Never hand-edit a generated file: `module_registry_gen.go`, `acl_gen.go`, `*.gen.go`,
  `ui-kit/types/api.d.ts`, the merged `backend/openapi.json`, `.ai/inventory.json`. Edit the
  source and run `make generate` / `make types` / `make inventory`. `make check` fails on a
  stale one.
- Never put anything but `name`, `description`, `depends`, `enabled` in `module.yaml`. The
  kernel's parser refuses a fifth key; routes, permissions and flags live in `module.go`.
- Never let `api.go` import beyond `httpx`, `i18n`, `command`, `platform`, `flags` and its own
  `contract/`, or use anything declared in `repository.go`, `handler.go` or `job.go`.
  Enforced by `thincontroller`.
- Never read a flag key that has no `flags.Definition` in the same package and on
  `Module.Flags`. An undeclared flag can never be switched off. Enforced by `flagdeclared`.
- Never add a translation key, or a landing page, in one locale without the other. The
  registry refuses to start on an API catalogue; `make i18n-check` fails on any of them.
- Never test a module's rules against a mock of a platform service. `platform.InMemory`
  gives every one of them for real.
- Never write a record and then return an error in the same handler - the transaction rolls
  the record back and the failure leaves no trace.
- Never take the tenant from the ambient scope on a WRITE when the row already knows it;
  pass it. Reads fail closed against the scope; writes made under `scope.Unscoped` must
  still name their tenant.
- Never call `fetch` or `$fetch` from a component - every request goes through `useApi()`,
  where the in-memory token, the single shared refresh on 401, cookie credentials and the
  typed 409 live.
- Never set `srcDir` or any other path in a module's `nuxt.config.ts` - a layer's config is
  merged into the app's and moves the app's own pages.
- Never hard-code a user-facing string or a domain. Everything derives from `DOMAIN` in `.env`.
- Never write a long dash (U+2014). Plain hyphen, everywhere. `make typography-check`.
- Never weaken a guardrail to make a change pass. Fix the change, or argue the rule should
  go and remove it outright - never mute it for one file.
- Never assert only a status code. Every check asserts content too.
- Never name a compose file in a script (read `COMPOSE_FILE` through `scripts/lib/compose.sh`);
  never `sleep` to wait - wait on a healthcheck; never `sed -i` under `scripts/dev` or
  `scripts/lib` - use `sed_inplace`.
- Never bump Postgres, Redis, Mercure, Go or Node in one file. Dev, production, the kernel's
  test containers, `backup-verify.sh`, `go.mod` and the Dockerfiles name the same version -
  enforced by `make versions-check`; Dependabot (`.github/dependabot.yml`) opens the PRs.
- Never let a build command mutate DNS or register a credential as a side effect;
  `dns-apply` and `deploy-key` are separate, deliberate acts.
- Never harden a server before proving you can still log in to it.
- Never regenerate a secret that already exists - in `.env` (`make env` fills only
  `__GENERATE__`) or on a server. `APP_SECRET` invalidates every session;
  `APP_ENCRYPTION_KEY` makes every encrypted column unreadable, permanently.
- Never commit a secret, a private key, a certificate or a `.env`.
- Never treat "the tool produced no output" as "the check passed".

---

## Validation commands

```bash
make check       # THE INNER LOOP - gofmt, vet, test -short, analyzers, generated-file freshness, docs, translations, budgets, typography, versions. ~15 s warm, no Docker
make ci          # check-tools + arch + selftest + test + typecheck + smoke + e2e - needs Docker and the stack up
make test-security  # the isolation and credential suite (backend/internal/security) - needs Docker
make test        # every Go test; starts one Postgres and one Redis through testcontainers
make arch        # every guardrail, dead code included; route coverage runs in `make ci`
make selftest    # every guardrail fails on a broken throwaway copy; a scaffolded module builds, serves and tests
make module NAME=x      # scaffold a module end to end, then generate, build and test it
make manager-module NAME=x  # give an existing module a layer in the operator console
make generate    # regenerate api.gen.go per module, module_registry_gen.go, backend/openapi.json
make types       # regenerate ui-kit/types/api.d.ts from backend/openapi.json
make modules     # what module discovery resolved
make check-tools # go, git, python3; docker, mkcert and openssl for the stack
make build       # backend/bin/openenu, static
make typecheck   # the three Nuxt apps against ui-kit/types/api.d.ts (in the node container)
make e2e         # Playwright against the running stack (SPEC=tests/x.spec.ts)
make i18n-check  # every translation key and landing page in both locales
make inventory   # regenerate .ai/inventory.json
make platform-update  # bring a project up to a newer platform; staged, never committed
make versions-check  # one Postgres, Redis, Mercure, Go and Node version in every file that names one
make preflight   # read-only: DNS, ports, SSH, repo access. Changes nothing
make builddev    # the whole stack up from nothing (~3 min cold); then make smoke, make urls, make down
```

Go is found by `scripts/lib/go.sh` in PATH, `~/.local/go` or `/usr/local/go`; nothing has to
be exported. A dev script resolves its root from its own location, so a selftest runs the
**copy's** scripts - see `.ai/platform/lessons/`. `make selftest` and `make platform-update`
make throwaway git repositories (with commits) in temporary directories; never this one.

If `make check` ever exceeds 60 seconds, treat it as a bug in the harness with the same
priority as a failing test: a slow inner loop stops being run.

---

## Task Router

Match the task to a row **before** researching or coding; a task often matches several.

| Task | Where to look |
|---|---|
| What the system is made of, the map of it | [`.ai/platform/ARCHITECTURE.md`](.ai/platform/ARCHITECTURE.md) |
| Why it is built this way; why a capability is deliberately absent | [`.ai/platform/adr/`](.ai/platform/adr/README.md) - 0011 (hierarchical orgs), 0012 (custom fields), 0014 (undo) |
| **Extending without modifying** - the named slots, reaching another module, what cannot be extended | [`extension-surfaces.md`](.ai/platform/docs/extension-surfaces.md) + [ADR-0005](.ai/platform/adr/0005-extension-by-named-slots.md) |
| A step-by-step procedure for a whole task | [`.ai/platform/skills/`](.ai/platform/skills/README.md) - create a module, add an endpoint, review a change |
| **Adding or changing a module** | [`module-development.md`](.ai/platform/docs/module-development.md) - start with `make module NAME=x` |
| Giving a module operator screens, or a card on the operator's tenant page | `make manager-module NAME=x`, then `module-development.md` → *Screens for an operator* |
| The shape of a module | `ARCHITECTURE.md` §4, then `backend/internal/module/example/` - one of everything, the thing to copy |
| **Writing any feature at all** | `backend/internal/module/example/` - read `MODULE.md`, copy `internal/` file by file |
| Authentication, the principal, API keys, the impersonation guard | [`auth.md`](.ai/platform/docs/auth.md) + ADR-0018 to ADR-0020 |
| Signing people in another way - single sign-on, a mailed link, a replaced system's login | `auth.md` → *Other ways to sign in*; Identity's `contract.Minter.StartSession` |
| A background job, a worker, a progress bar | `example/internal/job.go` + [`events-and-outbox.md`](.ai/platform/docs/events-and-outbox.md) → *Jobs* |
| Optimistic locking, `If-Match`, the 409 body | `example/internal/handler.go` `rename` + [`commands-and-audit.md`](.ai/platform/docs/commands-and-audit.md) |
| Declaring a feature flag so an operator can switch it | `example/internal/flags.go`, read with the same literal in `api.go` |
| Adding an endpoint; the spec, fragments, the generated server | [`api-contract.md`](.ai/platform/docs/api-contract.md) + [ADR-0006](.ai/platform/adr/0006-spec-first-openapi.md) |
| **Anything needing infrastructure** (flags, cache, storage, encryption, progress, events, audit, setup, GDPR) | [`platform-services.md`](.ai/platform/docs/platform-services.md) - the kernel owns the contract; `ARCHITECTURE.md` §5 is the map |
| Declaring a feature flag; why a route 404s behind one | `platform-services.md` → *Feature flags*; `kernel/flags` |
| Any user-facing string; catalogues; locale resolution; adding a locale | [`i18n.md`](.ai/platform/docs/i18n.md); `kernel/i18n`, `make i18n-check` |
| Why a guardrail is failing, and what it wants instead | the message names the alternative; then [`guardrails.md`](.ai/platform/docs/guardrails.md) |
| Error envelope, 409 body, request id, health probes | `backend/kernel/httpx/` |
| Logs, the request id a job carries, queues, "is anything stuck?" | [`observability.md`](.ai/platform/docs/observability.md); `kernel/logctx`, `Module.LogFields` |
| Passwords, the hasher, rehash on login | `kernel/crypto/password.go` + [ADR-0022](.ai/platform/adr/0022-argon2id-passwords.md) |
| What module discovery resolved | `make modules` |
| Tenancy, RLS, `scope.Unscoped`, the application role | [`tenancy.md`](.ai/platform/docs/tenancy.md) + [ADR-0009](.ai/platform/adr/0009-tenancy-by-row-level-security.md) |
| Writing data; the bus; what the trail records; 409 | [`commands-and-audit.md`](.ai/platform/docs/commands-and-audit.md) + [ADR-0013](.ai/platform/adr/0013-command-bus-with-explicit-snapshots.md) |
| Announcing something happened; background jobs; retries | [`events-and-outbox.md`](.ai/platform/docs/events-and-outbox.md) |
| Writing a test; which suite; the containers | [`testing.md`](.ai/platform/docs/testing.md) |
| Migrations; `openenu migrate`; the admin DSN | `backend/kernel/migrate/`, `backend/cmd/openenu/migrate.go` |
| **The dev stack** - services, env, air, `make builddev`, something is broken | [`dev-stack.md`](.ai/platform/docs/dev-stack.md); `docker/compose.dev.yml` |
| A service or route of the project's own, not the platform's | `docker/project/compose.<stage file>.yml` - `deployment.md` → *A project's own services and routes* |
| Live updates in a browser; the Mercure cookie; the hub | `backend/kernel/realtime/` + `ui-kit/app/composables/useMercure.ts` |
| Work on a clock | `module.Task` on the module (`backend/kernel/module/task.go`); `openenu scheduler` runs it |
| CORS, which origins may call the API | `httpx.CORS` in `backend/cmd/openenu/api.go` - `APP_URL` and `MANAGER_URL`, exact ([ADR-0021](.ai/platform/adr/0021-exact-origin-cors.md)) |
| Routing and TLS in dev; two projects on one Docker host | `docker/edge/` + `scripts/dev/edge.sh` - one shared Traefik; routers named `${STACK}-<service>` |
| **Anything in the three Nuxt apps** | [`frontend-conventions.md`](.ai/platform/docs/frontend-conventions.md) - then copy `frontend/app/modules/example/pages/example/index.vue` |
| Calling the API from the browser; 401 refresh; typed errors | `ui-kit/app/composables/useApi.ts` |
| Login, tenant switching, what the session holds | `ui-kit/app/stores/auth.ts` + `ui-kit/app/composables/useAuth.ts` |
| Browser-only behaviour - CORS, cookies, SSE, uploads | `e2e/tests/` + [`testing.md`](.ai/platform/docs/testing.md) |
| The landing site - copy, sections, pricing, legal pages | [`landing.md`](.ai/platform/docs/landing.md) - words live in `landing/content/{en,pl}/*.md` |
| **Deploying anything, anywhere**; staging and production; `make promote`; backups | [`deployment.md`](.ai/platform/docs/deployment.md) + `scripts/lib/stage.sh` |
| DNS, certificates, deploy keys, "it will not provision" | `scripts/remote/preflight.sh` - the failure message names the fix |
| What the production stack may and may not contain | `scripts/dev/check-prod-compose.sh` (`make prod-check`) |
| The production container's healthcheck; distroless has no shell | `openenu health` in `backend/cmd/openenu/health.go` |
| **The inventory, the MCP server, what an agent is offered**; route coverage; dead code | [`agent-interfaces.md`](.ai/platform/docs/agent-interfaces.md) |
| "Does something like this already exist?" | grep [`.ai/inventory.json`](.ai/inventory.json) - every declared name, regenerated by `make inventory` |
| Which permission an operation needs; why an endpoint is not an MCP tool | `backend/cmd/openenu/acl_gen.go` (generated from `auth.Require` in `api.go`) + `kernel/mcp/catalogue.go` |
| An operation `make route-coverage` says no test reached | write the test; a genuine exemption is `"x-no-test-required": "why"` on the operation in the module's `openapi.json` |
| CI, the Actions cache, "it passes locally and fails on GitHub" | `.github/workflows/ci.yml` + `docker/compose.ci.yml` - CI runs the same `make ci` |
| A mistake that has been made before | [`.ai/platform/lessons/`](.ai/platform/lessons/README.md) (and a project's `.ai/lessons/`) - read the index, never bulk-read the files |
| Where the platform's records end and the project's begin | [`.ai/README.md`](.ai/README.md) |
| Updating a project from the platform; fixing the platform from a project | [`updating.md`](.ai/platform/docs/updating.md) - `make platform-update` |
| Example - example | `backend/internal/module/example/MODULE.md` |
| Tenant - tenant | `backend/internal/module/tenant/MODULE.md` |
| Identity - identity | `backend/internal/module/identity/MODULE.md` |
| ApiKey - api_key | `backend/internal/module/api_key/MODULE.md` |
| Audit - audit | `backend/internal/module/audit/MODULE.md` |
| Attachment - attachment | `backend/internal/module/attachment/MODULE.md` |
| Progress - progress | `backend/internal/module/progress/MODULE.md` |
| Settings - settings | `backend/internal/module/settings/MODULE.md` |
| Manager - manager | `backend/internal/module/manager/MODULE.md` |
| Notification - notification; telling a person something happened; the feed; mail behind a type | `backend/internal/module/notification/MODULE.md` - raise one with `svc.Notifier`, declare the type on YOUR module (`Module.Notifications`) |
| Webhook - webhook; outbound webhooks, signing, delivery retries, the delivery log | `backend/internal/module/webhook/MODULE.md` + `internal/signer.go` (a fixed test vector in `signer_test.go`) |
| Search - search; the one `GET /api/search`; why a type needs a permission | `backend/internal/module/search/MODULE.md` - to make data findable, declare it on the owning module (`Module.Search`) |

---

## Repository map

```
backend/kernel/            github.com/open-enu/kernel - the framework. Versioned, never renamed.
backend/internal/module/   one Go package per feature; contract/ is public, internal/ is not
backend/internal/security/ the cross-module isolation and credential suite
backend/cmd/openenu/       one binary: api | worker | scheduler | migrate | mcp | inventory | health | console
backend/openapi.json       the contract - authored per module, merged, committed
ui-kit/                    @open-enu/ui-kit - the Nuxt layer the three apps extend
frontend/                  app.${DOMAIN}      tenant users
manager/                   manager.${DOMAIN}  platform operators (separate bundle and realm)
landing/                   ${DOMAIN}          marketing site
e2e/                       Playwright - what functional tests cannot see
docker/                    compose files, the api and node images, Postgres init, the edge
scripts/lib/ dev/ remote/  shared shell library, local tooling, provisioning
.ai/platform/              this platform's architecture, ADRs, docs, lessons, skills - replaced by an update
.ai/                       the project's own records - never touched by an update
```

### Where code goes

- Framework, used by every module, knows nothing about the app → `backend/kernel/`
- A feature → `backend/internal/module/<name>/` **and** `frontend/app/modules/<name>/`, one to one
- Shared UI, the API client, platform composables → `ui-kit/`
- Anything project-specific in the kernel → **wrong place**; it belongs in a module

## Core principles

- **Simplicity first.** The smallest change that fully solves the problem.
- **Find the root cause.** No temporary fixes, no symptom patches.
- **One module per change.** If it spreads, stop and ask.
- **Report honestly.** If tests fail, say so with the output. If a step was skipped, say so.
