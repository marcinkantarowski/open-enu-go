# The API contract

`backend/openapi.json` is **authored, then generated from**
([ADR-0006](../adr/0006-spec-first-openapi.md)). The frontends' TypeScript types, the MCP
tool catalogue and the route-coverage check all read it, so it has to be the truth, and the
server is generated from it so it cannot be anything else.

## Where the spec lives

| File | Owner | Role |
|---|---|---|
| `backend/internal/module/<name>/openapi.json` | the module | its paths and the schemas only it uses |
| `backend/kernel/httpx/components.json` | the kernel | shared schemas (`Error`, `ConflictDetails`), responses, security schemes |
| `backend/openapi.json` | **generated** by `tools/genspec` | the merged contract; committed; what everyone else reads |

A fragment carries `openapi`, `paths` and `components` and nothing else. It may
`$ref: '#/components/schemas/Error'` and `'#/components/responses/Error'` without defining
them - the merge supplies them. Two definitions of the same component name must be
identical, or the merge fails naming both owners.

## How a module gets a server

`go generate` in the module runs `tools/genapi`: fragment + shared components into a temp
file, then `oapi-codegen` (pinned as a `go tool` in `go.mod`, run as `go tool oapi-codegen`)
with `strict-server` and `std-http-server`, writing `internal/api.gen.go`:

- one Go type per schema (`BillingIndex`, `CreateInvoiceRequest`, …);
- `StrictServerInterface` with one method per operation, named from the operation id
  (`get_billing_index` → `GetBillingIndex`), taking a typed request object and returning a
  typed response object;
- the wiring that decodes requests, routes through `net/http`'s mux and encodes responses.

`internal/api.go` implements the interface. A method missing, misnamed or returning the
wrong type is a compile error - that is the contract enforced, at build time.

Errors: a handler returns `*httpx.Error` and the wiring (`Mount` in the scaffold) sends it
through `httpx.WriteError`, so the envelope is the same whether the failure came from the
handler, from decoding or from the kernel.

## Conventions

- **Operation ids:** `<verb>_<module>_<entity>_<action>` - `post_example_project_create`.
  Route coverage, the inventory and the MCP tool list all key on them.
- **The error envelope** `{"error": {code, message, details?, requestId?}}` - every
  non-2xx response, and the only shape `useApi` understands.
- **409** carries `ConflictDetails` in `error.details`.
- **Lists** return `{items, meta: {total, page, size, pages}}`, filled by
  `httpx.Paginate(...).Meta(total)`. Each
  module's fragment declares its own `ListMeta`; `kernel/httpx/components.json` shares only
  `Error` and `ConflictDetails`.
- **One security scheme**, `bearer`: a JWT or an API key, told apart by the key's `sk_`
  prefix; the realm is the JWT's audience.
- `/health` and `/health/deep` are kernel routes outside the spec.

## The frontends' types

Every operation has request and response schemas, so the spec can be typed against.
`make types` generates `ui-kit/types/api.d.ts` from `backend/openapi.json` and ends it with
the spec's sha256; `make gen-check` (`scripts/dev/check-types.sh`) fails when the spec has
changed since. No app imports the file yet - the composables pass their own type
parameters - so it is the shapes on offer, kept honest, not yet a constraint on the apps.

## Checks

- `make gen-check` (in `make check`): `api.gen.go`, `module_registry_gen.go` and
  `backend/openapi.json` are regenerated into a temp file and diffed. A fragment edited
  without `make generate` fails.
- `make selftest` renames a scaffolded path in its copy and proves both diffs fire.
