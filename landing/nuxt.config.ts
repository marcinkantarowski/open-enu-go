// =============================================================================
//  landing - the marketing site, served at ${DOMAIN} and www.${DOMAIN}.
// =============================================================================
//  The opposite of the two SPAs on every count: public, indexable, and with no
//  session to render, so it renders on the server. It also calls no API - the
//  marketing site stays up when the backend does not (.ai/platform/docs/landing.md).
//
//  Copy lives in content/{en,pl}/*.md, never in templates: a new project edits
//  markdown rather than components. Interface chrome (navigation, footer) is in
//  the i18n catalogue like every other user-facing string.
// =============================================================================

export default defineNuxtConfig({
  /**
   * The marker the smoke test looks for - a ROLE, never the project name, which
   * `make init` rewrites (.ai/platform/lessons/sentinel-strings-must-not-be-renameable.md).
   */
  app: {
    head: {
      meta: [{ name: 'app-role', content: 'marketing' }],
      link: [{ rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }],
    },
  },

  // Pinned, as in the other two apps: layer configs merge into this one.
  srcDir: 'app',

  extends: ['@open-enu/ui-kit'],

  modules: ['@nuxt/content'],

  css: ['~/assets/css/prose.css'],

  i18n: {
    // One URL per language - `/contact` and `/pl/contact` - unlike the two SPAs,
    // which inherit `no_prefix` from the ui-kit layer. A crawler sends neither a
    // cookie nor Accept-Language, so a language that shares its URL with another
    // is never indexed, and hreflang has nothing to point at.
    strategy: 'prefix_except_default',
    // No `langDir` - see .ai/platform/docs/i18n.md.
    locales: [
      { code: 'en', language: 'en-GB', files: ['en.ts'] },
      { code: 'pl', language: 'pl-PL', files: ['pl.ts'] },
    ],
  },

  runtimeConfig: {
    public: {
      appName: process.env.NUXT_PUBLIC_APP_NAME || 'OpenEnu',
      domain: process.env.NUXT_PUBLIC_DOMAIN || 'open-enu-go.local',
      // Where "Sign in" and "Get started" go. The site links to the app; it
      // never embeds it.
      appUrl: process.env.NUXT_PUBLIC_APP_URL || 'https://app.open-enu-go.local',
      siteUrl: process.env.NUXT_PUBLIC_SITE_URL || 'https://open-enu-go.local',
      // May search engines index this deployment? `envgen` says yes for
      // APP_STAGE=production and nowhere else. False by default, so a missing
      // variable hides a site rather than publishing a staging server.
      indexable: process.env.NUXT_PUBLIC_INDEXABLE === 'true',
      // The language served at the bare URL; every other one gets a prefix.
      // The sitemap, llms.txt and x-default all have to agree with the router
      // about which that is, so they read it from here.
      defaultLocale: process.env.NUXT_PUBLIC_DEFAULT_LOCALE || 'en',
    },
  },
})
