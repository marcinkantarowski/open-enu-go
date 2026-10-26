# ADR-0002 - The kernel is a package; its name is never the project's name

**Status:** accepted · **Date:** 2026-10-22

**This is the most important invariant in the repository.**

## Context
Projects are started from this repository and renamed with `make init`, and they must be
free to diverge. Two distribution models were available:

- **A versioned framework package** that is never forked, with the project's code beside
  it. Fixes arrive by updating a dependency.
- **Boilerplate as a template:** copy it, rename it, own it. Simple - and fixes to the
  boilerplate never reach the projects built from it.

The template model fits a foundation that projects reshape freely. But if the rename also
reaches the framework's own identifiers, no framework fix can ever be applied to a project
mechanically again.

## Decision
Framework and application are split at the package boundary:

| | Package | Renamed by `make init`? |
|---|---|---|
| Framework | `github.com/open-enu/kernel` - `backend/kernel/`, its own `go.mod` | **never** |
| Framework UI | `@open-enu/ui-kit` - `ui-kit/` | **never** |
| Application | `github.com/open-enu/app` -> `GO_MODULE`, else `github.com/<slug>/app` | yes |

The application's `go.mod` requires the kernel through a `replace` directive pointing at
`./kernel`: a subdirectory today, a tagged module whenever a project wants one.

`scripts/dev/init.sh` replaces exact strings in an enumerated set of places - the manifest,
`go.mod` and the application's import lines, `.env.example`, the compose files' `${STACK:-}`
fallbacks, the cache prefix default, the `AGENTS.md` title - and nothing pattern-based. It
refuses a `GO_MODULE` equal to the kernel's path.

Every other name the framework defines - cookies (`open_enu_refresh`, `open_enu_locale`),
the field-encryption derivation context (`open-enu:field:v1`) - contains neither the
project's slug nor its name, so `init` cannot reach it. It must not: renaming the
derivation context would make every encrypted column unreadable.

Nothing in `backend/kernel/` imports the application. The kernel's `go.mod` does not
require it, so such an import does not build.

## Updates
`make platform-update` brings a project up to a newer platform as a three-way merge between
two versions of the platform, each first run through `init` with the project's name, domain
and Go module path ([`docs/updating.md`](../docs/updating.md)). The kernel's names are the
part of the tree `init` never rewrites, so they never conflict. `init` records the platform
commit and source in `.project.json` (`platform_ref`, `platform_source`).

## Consequences
- Today: no cost. A `replace` to a subdirectory behaves exactly like a subdirectory.
- Later: a project may drop the `replace` and `go get` a tagged kernel.
- The invariant is fragile in exactly one place, `init.sh`, and is guarded by
  `make selftest`: it renames a throwaway copy, counts every framework identifier before
  and after, builds and tests the result, and proves an import of the application from the
  kernel fails to build. It also runs `platform-update` through a clean update, a conflict
  (reported, never chosen), a dirty tree (refused) and a base whose `init` cannot run.
- The discipline that application concerns never leak into the kernel.
