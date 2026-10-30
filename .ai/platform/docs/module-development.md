# Module development

A module is a Go package under `backend/internal/module/<name>/` with a `module.yaml`. That
is the whole definition: the generator finds it, the registry validates it, the server
mounts it. Nothing central is edited by hand.

**Start with `make module NAME=<name>`.** It writes the shape below and the frontend layer
`frontend/app/modules/<kebab-name>/` (one permission-gated page, a menu entry, both locales;
`bash scripts/dev/module.sh --no-frontend` skips it), generates the server from the spec
fragment, builds and runs the module's own test, adds the spec stub and the Task Router
row, and restarts the frontend container when it serves this checkout - Nuxt discovers a
new layer only at startup. The new route changes the contract, so `make types inventory`
follows. A module written by hand differs in ways nobody notices until the third one.

## The shape

```
backend/internal/module/<name>/
├── module.yaml        name, description, depends, enabled - and nothing else
├── module.go          var Module = module.Module{...}: the ONLY thing the registry reads
├── openapi.json       this module's paths and schemas (ADR-0006)
├── MODULE.md          what it owns, its surface, its public contract
├── contract/          what OTHER modules may import: events, ids, read ports
├── internal/          everything else - the compiler refuses it from outside
│   ├── api.gen.go     GENERATED from openapi.json: models + StrictServerInterface
│   ├── api.go         implements the interface; thin (see guardrails.md)
│   ├── mount.go       Mount: repository, handlers on the bus, jobs on the outbox, the controller
│   ├── command.go     the commands - the intent, as data
│   ├── handler.go     the only code that receives *scope.Tx
│   ├── repository.go  the only file that runs SQL or opens a read transaction
│   ├── entity.go      the row as the module reasons about it; Snapshot(); view()/detail()
│   ├── voter.go       the record-level check the handler consults
│   ├── job.go         background work: a River job and its worker
│   ├── flags.go       the module's flags.Definition literals
│   ├── setup.go       the tenant seed
│   └── fixtures.go    deterministic rows for tests (`LoadFixtures`, exported from module.go)
├── migrations/        goose SQL; a tenant-scoped table ends with the RLS policy
├── i18n/              messages.en.json, messages.pl.json
└── <name>_test.go     every route through httptest - as a person, as a key, as nobody

`backend/internal/module/example/` has one of each and is the thing to copy.
```

### `module.yaml` - four keys

```yaml
name: billing            # == the directory, the Go package, the permission prefix
description: One line.
depends: []              # modules whose contract/ this one may import
enabled: true
```

The kernel reads it with its own strict parser (`kernel/module.ParseManifest`): an unknown
key is refused with a message saying where the thing belongs. Routes, permissions, flags and
settings are declared in `module.go`, never here - a manifest that can hold structure turns
into a schema.

### `module.go` - the declaration

```go
//go:embed module.yaml
var manifest []byte
//go:embed openapi.json
var spec []byte
//go:embed i18n
var locales embed.FS

var Module = module.Module{
	Manifest:    module.MustManifest(manifest), // one source: the file
	Permissions: []string{"billing.view", "billing.manage"},
	Mount:       internal.Mount, // func(mux *http.ServeMux, svc *platform.Services)
	Locales:     module.MustSub(locales, "i18n"),
	OpenAPI:     spec,
	Flags:       internal.Flags,   // []flags.Definition - what this module reads
	Setup:       internal.Setup{}, // setup.TenantSetup, when the module needs rows per tenant
	Gdpr:        internal.Gdpr{},  // gdpr.Subject, when the module holds data about a person
	Tasks:       internal.Tasks,   // []module.Task - work on a clock, run by `openenu scheduler`
	Search:      internal.Searchable, // []search.Declaration - what is findable, and the permission that gates it
	Notifications: internal.Notifications, // []notification.Definition - what this module may tell people, keys in its own catalogue
	LogFields:   []logctx.Field{internal.LogField}, // what this module adds to every log line, read from the context
}
```

Every field and what it costs is in [`extension-surfaces.md`](extension-surfaces.md); a
module extends the platform through these and nothing else
([ADR-0005](../adr/0005-extension-by-named-slots.md)).

`Mount` receives the platform - flags, cache, storage, progress, events, audit, crypto,
the catalogue ([`platform-services.md`](platform-services.md)) - and keeps what its handlers
need. A test mounts with `platform.InMemory(nil)`.

Permissions are global and prefixed with the owner. `<name>.view` - and `<name>.use`, for a
module's everyday work - is what every member holds; anything else needs a role somebody
granted (owner and admin; the rule is `identity/internal/permissions.go`). `<name>.review` is
a check by someone other than the workspace's owner: an admin holds it, the owner does not.
The registry refuses a permission without the prefix and a string declared twice.

### Visibility is the module boundary

Another module may import `billing/contract` and nothing else. `billing/internal` is
refused by the compiler - Go's `internal` rule, no analyzer needed. Reaching another
module's data means a read port in its `contract/`, or an event; never its repository.
Everything exported from `contract/` must be named in `MODULE.md` (`make docs-check`).

### Adding a route

1. Add the operation to `openapi.json` - paths **and** schemas; the spec is authored, not
   generated ([ADR-0006](../adr/0006-spec-first-openapi.md), [`api-contract.md`](api-contract.md)).
2. `go generate ./` in the module (or `make generate`). `api.gen.go` now has a method the
   interface demands; the build fails until `api.go` provides it.
3. Implement it in `api.go`: decode nothing by hand, dispatch a command, map the result to
   the generated response type. Errors are `*httpx.Error` with a translation key.
4. Add the route to `<name>_test.go`. `make route-coverage` (in `make ci`) fails on an
   operation no test reached.
5. `make check`.

### Screens for an operator

Most features have nothing for an operator to do, so `make module` writes no operator half.
When one does - a balance to credit, a request to approve - `make manager-module NAME=billing`
gives the existing module a layer in the console, discovered by `manager/nuxt.config.ts` as
the tenant app discovers its own:

```
manager/app/modules/billing/
├── nuxt.config.ts       the layer manifest - i18n only, never paths
├── navigation.ts        defineNavigation([...]) - entries after the console's own; may be empty
├── injections.ts        defineInjection('manager.tenant', Card) - optional
├── pages/billing/       optional; the directory IS the URL
├── components/
└── i18n/locales/{en,pl}.{json,ts}
```

Same discovery, same rules, two differences. There is no `permission` to filter by: an operator
has a role, and the realm is the gate, so every endpoint a layer calls is `/api/manager/...`,
declared in the module's own `openapi.json` and starting with `auth.RequireOperator(ctx)`.
And the usual way in is not a page but a card on the tenant page:
`<InjectionPoint name="manager.tenant">` renders whatever modules registered and hands each
one `tenantId`. A tenant's balance, limits and pending requests then sit where the operator
already is, and the tenant page never learns which modules exist.

### What the registry checks at start-up

`module.NewRegistry` refuses, with a message naming the fix: a name that does not match
`^[a-z][a-z0-9_]*$`, a name declared twice, a dependency that is absent or disabled, a
dependency cycle, a permission or a flag without the owner's prefix or declared twice, a
catalogue key present in one locale and missing from the other.
`openenu modules` prints what it resolved - the first thing to look at when a route is
missing.

### Generated files

`api.gen.go`, `cmd/openenu/module_registry_gen.go` and `backend/openapi.json` say
`GENERATED` at the top. Never edit them; edit the source (the fragment, the manifest) and
run `make generate`. `make check` fails when any of them is stale.

### Persistence

Commands, handlers, `*scope.Tx`, the repository and the entity are the kernel's
([`commands-and-audit.md`](commands-and-audit.md), [`tenancy.md`](tenancy.md)). The
scaffold's `api.go` answers without a database until `Mount` wires a repository and
handlers; copy `backend/internal/module/example/internal/` for the shape. Do not improvise
persistence outside that shape. Everything else a module needs from the platform is
already there - [`platform-services.md`](platform-services.md).
