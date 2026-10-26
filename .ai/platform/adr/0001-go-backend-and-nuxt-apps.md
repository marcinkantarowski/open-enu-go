# ADR-0001 - The backend is one Go binary; the browser apps are Nuxt

**Status:** accepted · **Date:** 2026-10-22

## Context
A SaaS foundation has two halves with different needs. The API must be small enough for one
person or one agent to hold in their head, deploy with as few moving parts as possible, and
make its rules checkable by tools. The browser side must be able to grow into a UI-heavy
product without fighting its framework.

## Decision
The API, the background worker and the scheduler are **one static Go binary**, `openenu`
(`backend/cmd/openenu/`): `api`, `worker`, `scheduler`, `migrate`, `mcp`, `inventory`,
`health` and the console commands are its subcommands. There is no ORM: each module writes
its SQL by hand in `repository.go` over `pgx`, Postgres enforces tenancy (ADR-0009), and
River - also on Postgres - is the job queue and the outbox (ADR-0015).

The browser apps are **Nuxt**: `frontend` (tenant users) and `manager` (platform operators)
are single-page apps, `landing` renders on the server from markdown. All three extend the
shared layer `@open-enu/ui-kit`.

The contract between the halves is `backend/openapi.json` (ADR-0006) and observable HTTP
behaviour; `make e2e` asserts that behaviour through a real browser against the real stack.

The kernel - everything every module uses - is its own Go module (ADR-0002).

## Considered and rejected: server-rendered HTML from the Go binary
`html/template`, `embed` and SSE from `net/http` could serve all three apps from the same
binary with a few hundred lines of plain JavaScript for the live feed, uploads and the
conflict bar; Node and the Mercure hub would leave the stack. Rejected: a UI-heavy product
is cheaper to build in Vue than in templates, and the JSON API is needed anyway - it is the
contract for machine clients and for the MCP server (ADR-0006). Revisit only if the
frontends become the maintenance cost, not on principle.

## Consequences
- Production runs one distroless image for `api`, `worker` and `scheduler`, plus one image
  per Nuxt app. No application server, no process supervisor.
- Two structural rules are compiler behaviour: module boundaries (`internal/` packages,
  ADR-0004) and kernel purity (the kernel's module has no dependency on the application's,
  ADR-0002). "Writes only in handlers" is mostly a type rule: only a handler receives a
  `*scope.Tx` (ADR-0013).
- Every other guardrail is a `go/analysis` analyzer under `backend/kernel/analysis/`, each
  with a fixture that breaks it, run by `make check` and `make arch` and proved to fail by
  `make selftest`.
- Without an ORM, CRUD code is longer and explicit. The `Example` module's line budget
  (1150 lines including tests, `make agents-budget`) is set from measurement.
- Go is installed natively, because `make check` must stay under 60 seconds; npm runs only
  in the node container.
