// A module is a Nuxt layer, so its pages, components and composables register
// themselves - and deleting this directory removes the feature cleanly, with no
// dangling imports left behind (ADR-0007).
//
// Imported rather than relied on as a global: this file lives under the app's
// srcDir, so it is type-checked in the app project, where Nuxt's config globals
// are not declared.
import { defineNuxtConfig } from 'nuxt/config'

export default defineNuxtConfig({
  // Deliberately no `srcDir`. `extends` merges a layer's config into the app, so
  // setting it here would silently become the APP's srcDir, and the app's own
  // pages/ and middleware/ would stop being scanned - no error, just a router
  // that matches nothing.

  i18n: {
    // `<module>/i18n/locales` is where @nuxtjs/i18n looks by default, so a
    // feature keeps its translations beside its pages rather than in one file
    // every branch edits. The loaders point at the .ts wrappers, never the
    // .json - see i18n/locales/en.ts for why.
    locales: [
      { code: 'en', files: ['en.ts'] },
      { code: 'pl', files: ['pl.ts'] },
    ],
  },
})
