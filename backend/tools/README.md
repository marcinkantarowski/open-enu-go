# tools - go:generate helpers

Four commands, each with a `-check` flag that exits 1 when the committed output is stale
instead of rewriting it. `make generate` runs them all; `make gen-check` (in `make arch`)
runs every `-check`.

Run from `cmd/openenu` (the three directives in `cmd/openenu/main.go`):

- `genregistry` - walks `internal/module/*/module.yaml`, writes `cmd/openenu/module_registry_gen.go`.
  This is how a new directory becomes a routed module with no central file edited by hand.
- `genspec` - merges `internal/module/*/openapi.json` with the kernel's shared components
  (`kernel/httpx/components.json`) into `backend/openapi.json` (ADR-0006) - the one file the
  frontends and the MCP server read.
- `genacl` - reads the `auth.Require(ctx, "...")` literal in each handler in every module's
  `internal/api.go` and writes `cmd/openenu/acl_gen.go`: operationId -> permission, the ACL the
  MCP catalogue filters by. An operation whose handler calls no `auth.Require` is absent.

Run from a module directory (the directive in its `module.go`, for every module with an
`openapi.json` - `tenant` has none):

- `genapi` - merges that module's fragment with the shared components and runs the pinned
  `oapi-codegen` (`go tool`) to write `internal/api.gen.go`: the models and a
  `StrictServerInterface` for this module's operations only. `internal/api.go` implements it,
  so a handler that disagrees with the spec does not compile.

`internal/spec` is the merge `genspec` and `genapi` share. There is no Go tool for the
TypeScript types: `make types` runs `openapi-typescript` over `backend/openapi.json` into
`ui-kit/types/api.d.ts` and stamps it with the spec's hash, which `make gen-check` compares.
