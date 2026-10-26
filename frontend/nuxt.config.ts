import { readdirSync, existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

// =============================================================================
//  frontend - the tenant application, served at app.${DOMAIN}.
// =============================================================================

const modulesDir = fileURLToPath(new URL('./app/modules', import.meta.url))

/**
 * Every module is a layer, discovered rather than listed.
 *
 * A hand-maintained array here would be one more file to edit when adding a
 * feature - and the one people forget, producing a module whose pages simply do
 * not exist with no error anywhere. Reading the directory makes `mkdir` the
 * whole registration step (ADR-0007).
 */
const moduleLayers = existsSync(modulesDir)
  ? readdirSync(modulesDir, { withFileTypes: true })
      .filter(entry => entry.isDirectory() && existsSync(`${modulesDir}/${entry.name}/nuxt.config.ts`))
      .map(entry => `./app/modules/${entry.name}`)
      .sort()
  : []

export default defineNuxtConfig({
  /**
   * The marker the smoke test looks for.
   *
   * `app.head` is baked into the HTML shell at build time, so it is present even
   * on the two apps that do not render on the server - which is the point: a
   * smoke test that only checks for HTTP 200 passes on an error page, and one
   * that checks for a page's own words breaks the moment somebody edits the
   * copy. This also proves each HOST reaches the app it is supposed to: three
   * routes pointing at one container would otherwise look perfectly healthy.
   *
   * The value is a role, never the project name - `make init` rewrites the
   * project name, and a check that compares against something the tool rewrites
   * inverts itself (see .ai/platform/lessons/sentinel-strings-must-not-be-renameable.md).
   */
  app: {
    head: {
      meta: [{ name: 'app-role', content: 'tenant' }],
    },
  },

  // Pinned rather than left to the default: layer configs merge into this one,
  // so a layer that sets `srcDir` would move it out from under the app.
  srcDir: 'app',

  // Everything shared - dev server, HMR over TLS, design tokens, Pinia, i18n -
  // comes from the layer. Module layers come after it so their pages and
  // components can use what it provides.
  extends: ['@open-enu/ui-kit', ...moduleLayers],

  /**
   * A single-page app, deliberately.
   *
   * The access token lives in memory and the refresh cookie is httpOnly and
   * host-scoped to the API, so a server render has no session to render WITH: it
   * would emit a signed-out shell that flips to signed-in on hydration. SSR here
   * would buy nothing - this is an authenticated application behind a login, not
   * a page anyone needs indexed. The landing site, which is, keeps SSR on.
   */
  ssr: false,

  i18n: {
    // This app's own strings. The layer ships the shared vocabulary; module
    // layers add theirs. `@nuxtjs/i18n` merges the `files` arrays by locale
    // code, which is what lets a feature keep its translations beside its pages
    // instead of in one file every branch edits.
    // No `langDir`: @nuxtjs/i18n resolves catalogues to `<layer>/i18n/locales`
    // by default, and every layer here follows that. Overriding it is how the
    // path ends up doubled - the module already joins its own `i18n/` prefix.
    locales: [
      { code: 'en', files: ['en.ts'] },
      { code: 'pl', files: ['pl.ts'] },
    ],
  },

  runtimeConfig: {
    public: {
      appName: process.env.NUXT_PUBLIC_APP_NAME || 'frontend',
      domain: process.env.NUXT_PUBLIC_DOMAIN || 'open-enu-go.local',
      apiBase: process.env.NUXT_PUBLIC_API_BASE || '',
      mercureUrl: process.env.NUXT_PUBLIC_MERCURE_URL || '',
      // The platform's own demonstration content - the reference module's menu
      // entry. On unless switched off: a fresh clone should show what it can
      // do, and a product built from it switches this off in `.env`.
      demoContent: process.env.NUXT_PUBLIC_DEMO_CONTENT !== 'false',
    },
  },
})
