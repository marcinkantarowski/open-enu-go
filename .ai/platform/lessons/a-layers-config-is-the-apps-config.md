---
tags: [nuxt, layers, configuration, silent-failure]
date: 2026-10-25
---
# A layer sets the app's config, not its own

Every frontend module is a Nuxt layer ([ADR-0007](../adr/0007-frontend-modules-as-layers.md)),
and `extends` merges a layer's config **into the app's**. A path set in a layer is not scoped
to the layer: it moves the app.

## The trap
Module layers keep their pages at `<module>/pages/`, not `<module>/app/pages/`. Reasoning that
Nuxt 4 defaults `srcDir` to `app/`, it is natural to give a layer:

```ts
export default defineNuxtConfig({ srcDir: '.' })
```

That does not scope `.` to the layer - it sets the application's `srcDir` to the
application's own root. The app's `pages/`, `layouts/`, `middleware/` and `plugins/` all live
under `app/`, so the scanner looks in a directory that contains none of them and finds nothing
to complain about. When several layers set it, the last one to merge wins. Nothing warns.

What it looks like: a router with nothing in it, not a 404 from the router.
`.nuxt/types/middleware.d.ts` reads `export type MiddlewareKey = never`, no `routes.mjs` is
generated, and the browser console says `No match found for location with path "/login"` for
a page that plainly exists on disk. `curl` returns 200 and a plausible HTML shell - the Nitro
server is fine - and the type-check passes, because a missing route is not a type error. A
browser test fails on `getByTestId('login-email')` timing out, which reads like a slow page
rather than like an empty router; the console warning is what names it.

## The rule
**Treat a layer's config as a patch applied to the app, not as settings for the layer.**
Anything path-shaped - `srcDir`, `dir.*`, `rootDir` - belongs to the app and is never set from
a layer.

## Where it is held
- Each app **pins** `srcDir: 'app'` in its own `nuxt.config.ts` (`frontend/`, `manager/`,
  `landing/`), so no layer can move it.
- The layers `make module` and `make manager-module` generate set no paths at all, and the
  template carries a comment saying why, because the natural instinct is to add one back.
- `make selftest` asserts that both generated layers contain no `srcDir:`.
- `AGENTS.md` (Never list) and `.ai/platform/skills/create-module.md` say it where an agent
  reads first.

## Where else this applies
- `app.head`, `i18n`, `runtimeConfig` and `css` all merge the same way - merging is the point
  of layers - but for those, merging is what you want. Paths are the exception.
- The same trap exists for any tool with layered config: a value that is relative to "here"
  means something different once "here" is the consumer's directory.
