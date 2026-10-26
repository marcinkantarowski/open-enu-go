# ADR-0007 - Each frontend module is a local Nuxt layer

**Status:** accepted · **Date:** 2026-10-22

## Context
A `modules/<name>/pages/` folder that Nuxt does not know about needs re-export shims in the
app's real `pages/` directory - boilerplate per page, and a dangling import every time a
module is deleted.

## Decision
Each `frontend/app/modules/<name>/` (and `manager/app/modules/<name>/`) has its own
`nuxt.config.ts` and is a Nuxt layer. The app's `nuxt.config.ts` discovers every module
directory that has one and lists it in `extends`, after `@open-enu/ui-kit`. Pages,
components and composables auto-register; navigation entries, settings tabs and
injection-point contributions are declared by the layer itself. `make module` creates the
tenant app's layer, `make manager-module` the operator console's.

A module's `nuxt.config.ts` never sets `srcDir` or any other path: a layer's config is
merged into the app's, so a path set there moves the app's own pages.

## Consequences
- Deleting a folder removes a feature cleanly.
- Backend and frontend module names match one to one, so paths stay derivable.
- Layer resolution order is meaningful - documented in
  [`docs/frontend-conventions.md`](../docs/frontend-conventions.md).
