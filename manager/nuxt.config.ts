
// =============================================================================
//  manager - the platform operator console, served at manager.${DOMAIN}.
// =============================================================================
//  A separate app and a separate bundle from `frontend`, deliberately. The two
//  realms share a signing key, so the `aud` claim is what isolates them
//  (ADR-0019) - but shipping one bundle that could talk to both would put the
//  operator's screens one router mistake away from a tenant's browser.
// =============================================================================

import { existsSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const modulesDir = fileURLToPath(new URL('./app/modules', import.meta.url))

/**
 * A module may bring operator screens, as a layer - discovered, never listed,
 * for the same reason as in the tenant app (ADR-0007). Most modules have none:
 * only a feature with something for an OPERATOR to do gets a directory here,
 * and `make manager-module NAME=X` is what creates it.
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
      meta: [{ name: 'app-role', content: 'operator' }],
    },
  },

  // Pinned rather than left to the default: layer configs merge into this one,
  // so a layer that sets `srcDir` would move it out from under the app.
  srcDir: 'app',

  // Module layers after the shared one, so their pages can use what it provides.
  extends: ['@open-enu/ui-kit', ...moduleLayers],

  // Same reasoning as the tenant app: the token is held in memory, so there is
  // no session to render on the server.
  ssr: false,

  i18n: {
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
      appName: process.env.NUXT_PUBLIC_APP_NAME || 'manager',
      domain: process.env.NUXT_PUBLIC_DOMAIN || 'open-enu-go.local',
      apiBase: process.env.NUXT_PUBLIC_API_BASE || '',
      mercureUrl: process.env.NUXT_PUBLIC_MERCURE_URL || '',
      // Where an impersonation handoff is sent. The operator console never
      // renders tenant screens itself; it opens the tenant app with a session.
      appUrl: process.env.NUXT_PUBLIC_APP_URL || '',
    },
  },
})
