# ADR-0003 - A modular backend, not flat layers

**Status:** accepted · **Date:** 2026-10-22

## Context
Flat layers (`controllers/`, `services/`, `entities/`, ...) spread one feature across ten
top-level directories. Whoever adds the tenth feature - a person or an agent - has to hold
all ten in context to keep them coherent.

## Decision
Every feature is a self-contained module, `backend/internal/module/<name>/`:

- `module.yaml` - `name`, `description`, `depends`, `enabled`, and nothing else;
- `module.go` - the `module.Module` value: routes, permissions, flags, search, tenant setup,
  tasks and the other named slots (ADR-0005). It is the only thing the registry sees;
- `contract/` - what other modules may import; `internal/` - everything else;
- `openapi.json` (its fragment of the contract, ADR-0006), `migrations/`, `i18n/`,
  `MODULE.md` and its tests.

Its browser half is `frontend/app/modules/<name>/`, one to one (ADR-0007).

Modules are discovered, never registered by hand: `tools/genregistry` reads every
`module.yaml` and writes `backend/cmd/openenu/module_registry_gen.go`; `depends` orders
mounting. `make module NAME=x` scaffolds a module across the backend, the frontend, the docs
and the Task Router, then generates, builds and tests it.

## Consequences
- A feature's blast radius is one directory; it can be held whole.
- Boundaries erode unless enforced: ADR-0004.
- Reaching another module costs more by design - its `contract/` or its events, never its
  internals.
