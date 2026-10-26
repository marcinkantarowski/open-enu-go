import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath } from 'node:url'

// =============================================================================
//  @open-enu/ui-kit - the Nuxt layer shared by frontend, manager and landing.
// =============================================================================
//  Everything the three apps must agree on lives here: the dev-server contract,
//  the design tokens, Pinia, i18n, and the API/auth/realtime composables that
//  `app/` exports (.ai/platform/docs/frontend-conventions.md).
//
//  An app's own nuxt.config holds only what makes it that app - its name, its
//  API base, whether it renders on the server.
// =============================================================================

const domain = process.env.NUXT_PUBLIC_DOMAIN || 'open-enu-go.local'

// Resolved against THIS file rather than the consuming app's directory.
// A layer's relative paths are resolved from the app that extends it, so a bare
// './app/assets/…' here silently points at frontend/ and the theme never loads.
const here = (path: string) => fileURLToPath(new URL(path, import.meta.url))

export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',

  modules: ['@pinia/nuxt', '@nuxtjs/i18n'],

  // One alias for everything this layer exports.
  //
  // Composables and components auto-import, but TYPES do not, and neither do
  // the Pinia stores from a plugin. A layer's alias merges into the app, so
  // `@ui-kit/stores/auth` means the same file in all three apps - and, unlike a
  // deep import through the package name, it does not depend on how the package
  // manager happened to link the workspace.
  alias: {
    '@ui-kit': here('./app'),
  },

  css: [here('./app/assets/css/main.css')],

  pinia: {
    // Layer-relative, for the same reason as `here()` above.
    storesDirs: [here('./app/stores/**')],
  },

  i18n: {
    // Two locales from day one, so adding a third is a file rather than a
    // project (ADR-0008). Each layer contributes its own catalogue; the module
    // merges the `files` arrays, which is what lets a feature ship its strings
    // next to its pages.
    // DEFAULT_LOCALE in the root .env, by way of the generated <app>/.env.
    defaultLocale: (process.env.NUXT_PUBLIC_DEFAULT_LOCALE || 'en') as 'en' | 'pl',
    strategy: 'no_prefix',
    // No `langDir`: @nuxtjs/i18n resolves catalogues to `<layer>/i18n/locales`
    // by default, and every layer here follows that. Overriding it is how the
    // path ends up doubled - the module already joins its own `i18n/` prefix.
    locales: [
      { code: 'en', language: 'en-GB', files: ['en.ts'] },
      { code: 'pl', language: 'pl-PL', files: ['pl.ts'] },
    ],
    detectBrowserLanguage: {
      useCookie: true,
      cookieKey: 'open_enu_locale',
      // Not `Strict`: the landing site links into the app across subdomains, and
      // a strict cookie would be dropped on exactly that navigation, resetting
      // the language every time someone signs in.
      cookieCrossOrigin: true,
      redirectOn: 'root',
    },
  },

  vite: {
    plugins: [tailwindcss()],

    server: {
      // Vite refuses requests for hosts it does not know, and every app here is
      // reached through Traefik under a custom name. The leading dot is Vite's
      // subdomain wildcard, so app./manager./anything.${DOMAIN} all pass without
      // this layer needing to know which app it is configuring.
      allowedHosts: [domain, `.${domain}`],

      // HMR is proxied through Traefik over TLS. Left at its defaults the
      // browser dials ws://<host>:3000 directly, which cannot connect - and the
      // failure is silent: the page simply stops hot-reloading.
      hmr: {
        protocol: process.env.VITE_HMR_PROTOCOL || 'ws',
        host: process.env.VITE_HMR_HOST || 'localhost',
        clientPort: Number(process.env.VITE_HMR_PORT || 24678),
      },

      // Bind-mounted source on WSL2 and macOS does not deliver inotify events
      // reliably into the container; polling is the cost of hot reload working.
      watch: { usePolling: true, interval: 300 },
    },
  },

  devServer: {
    host: '0.0.0.0',
    port: 3000,
  },

  typescript: { strict: true },
})
