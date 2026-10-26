# backend

Two Go modules and one contract.

- `kernel/` - `github.com/open-enu/kernel`. The framework. Knows nothing about the app.
- `internal/module/<name>/` - the application, one package per feature
  (`.ai/platform/ARCHITECTURE.md` §4).
- `cmd/openenu/` - the one binary. Servers: `api`, `worker`, `scheduler`, `mcp`. Operations:
  `migrate`, `modules`, `health`, `inventory`, `route-coverage`. Console verbs: `user`,
  `tenant`, `apikey`, `manager`, `attachment`. Modules are scaffolded by `make module NAME=X`,
  not by the binary.
- `openapi.json` - GENERATED: the merged contract, from `internal/module/*/openapi.json`
  fragments plus `kernel/httpx/components.json` (ADR-0006, `tools/genspec`). Edit a fragment,
  then `make generate`.
- Migrations: the kernel's own tables in `kernel/migrate/migrations/`, each module's in its
  `migrations/`; `openenu migrate` runs them in dependency order. The database roles and
  extensions are created by `docker/postgres/init/01-init.sh`, before any migration.
- `internal/security/` - tenant and realm isolation, the cross-module suite (`make test-security`).
- `tools/` - `go:generate` helpers: `genregistry` (module registry), `genspec` (merged spec),
  `genacl` (operation id to permission, for the MCP server), `genapi` (per-module strict
  server via the pinned `oapi-codegen` tool).
