# ADR-0027 - The platform's knowledge lives in `.ai/platform/`; the rest of `.ai/` is the project's

**Status:** accepted · **Date:** 2026-10-22

## Context
This repository is a platform that projects are started from, and `make platform-update`
brings a project up to a newer platform (ADR-0002). That covers code; it must also cover
`.ai/`, which is where an agent learns how the platform works and what a project decided.

Left as one tree, `.ai/` makes an update impossible in practice. A project adds its own
decision as `adr/0028`; the platform ships its own `0028`. A project appends a row to the
lessons index; the platform appends one too. Every index becomes a merge conflict, and the
two kinds of knowledge - *how the platform works* and *what this project decided* - blur
until nobody can say which records an update may replace.

## Decision
One boundary: **`.ai/platform/` ships with the platform; everything else under `.ai/`
belongs to the project.** `platform/` holds `ARCHITECTURE.md`, `adr/`, `specs/`, `docs/`,
`skills/`, `lessons/` and `analysis/`. The project side mirrors it - `.ai/adr/`,
`.ai/specs/`, `.ai/lessons/`, `.ai/analysis/`, each with its own index - and a project's
ADRs are numbered from 0001, independently of the platform's.

Which side a new record belongs to is read from `"initialized"` in `.project.json`, never
judged: `false` is the platform's repository, `true` is a project. `make module` writes its
spec stub, with its row in the index, on the right side.

## Considered and rejected
- **A `platform/` subdirectory inside each kind** (`lessons/platform/`, ...): the boundary
  repeated in every directory, and nothing single an update could replace.
- **One `.ai/project/` directory for the project:** one boundary, the wrong way round - the
  project, which is what people work in every day, would get the longer paths.

## Consequences
- A platform update replaces `.ai/platform/`. Nothing a project wrote is in the way.
- A project works at the natural paths: its spec is `.ai/specs/...`, its decision
  `.ai/adr/...`.
- In the platform's repository the project side stays empty apart from its indexes and the
  generated inventory - anything else would be handed to every new project as its own.
  `make docs-check` fails otherwise, and fails on any record missing from its index. Both
  are proved to fail in `make selftest`.
- A project never edits a platform record. To depart from one it writes its own ADR naming
  it, so the departure survives the next update.
- `AGENTS.md` is one shared file - `make module` adds Task Router rows to it - so it is the
  one place an update must be merged by hand.
