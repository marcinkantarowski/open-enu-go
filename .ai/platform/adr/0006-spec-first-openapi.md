# ADR-0006 - `openapi.json` is the contract: authored first, the server generated from it

**Status:** accepted · **Date:** 2026-10-22

## Context
API documentation treated as optional drifts immediately. This repository also *relies* on
its spec: the frontends' TypeScript types are generated from it, the MCP server offers its
operations as tools, route coverage is measured against it, and an agent reads it to learn
what exists. A spec derived from code is only as good as the annotations it was derived
from, and it arrives after the code it describes.

## Decision
The spec is the source, and code is generated from it.

- Each module authors its fragment, `backend/internal/module/<name>/openapi.json`; shared
  schemas live in `backend/kernel/httpx/components.json`. `tools/genspec` merges them into
  `backend/openapi.json`, which is committed - one file for the frontends, CI and agents.
- `oapi-codegen` generates a strict server interface per module (`internal/api.gen.go`);
  the module's `api.go` implements it. A handler that disagrees with the spec is a compile
  error.
- `make types` generates `ui-kit/types/api.d.ts` from the merged spec and stamps it with the
  spec's hash; a backend change that breaks a frontend is a type error.
- `make check` fails when any generated file is stale (`gen-check`); `openapi-check`
  validates the spec itself.
- Every operation needs a test that reaches it: the test server traces each matched route
  (`ROUTE_TRACE`), and `make route-coverage` fails on an operation no test reached. A genuine
  exemption is `"x-no-test-required": "<why>"` on the operation.

## Consequences
- Adding an endpoint starts in the fragment, then `make generate`, then `api.go`
  ([`docs/api-contract.md`](../docs/api-contract.md)).
- A contract change is visible as a diff in `backend/openapi.json` and `api.d.ts`.
- Generated files are never edited by hand.
- The spec carries what the generator and the frontends need: typed schemas, operation
  ids, error responses.
