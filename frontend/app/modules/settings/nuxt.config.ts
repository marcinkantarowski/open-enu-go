// A module is a Nuxt layer, so its pages, components and composables register
// themselves - and deleting this directory removes the feature cleanly, with no
// dangling imports left behind (ADR-0007).
import { defineNuxtConfig } from 'nuxt/config'

export default defineNuxtConfig({
  // Deliberately no `srcDir` here. `extends` merges a layer's config into the
  // app, so setting it in a module would silently become the APP's srcDir - and
  // the app's own pages/ and middleware/ would stop being scanned, with no error
  // anywhere, just a router that matches nothing.

  i18n: {
    // Catalogues live at `<module>/i18n/locales`, which is where @nuxtjs/i18n
    // looks by default - so a feature keeps its translations beside its pages
    // instead of in one file every branch edits.
    locales: [
      { code: 'en', files: ['en.ts'] },
      { code: 'pl', files: ['pl.ts'] },
    ],
  },
})
