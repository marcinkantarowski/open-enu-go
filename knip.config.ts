import type { KnipConfig } from 'knip'

// =============================================================================
//  Dead code, for the TypeScript half (.ai/platform/docs/agent-interfaces.md - blocking).
// =============================================================================
//  The failure mode is ABANDONED SCAFFOLDING: generated files nothing imports,
//  a composable replaced but not deleted, a dependency added for an approach
//  that was dropped. None of it breaks anything, which is exactly why it stays.
//
//  A TypeScript config rather than `knip.json`, because every exception below
//  needs a reason attached to it. An unexplained ignore is indistinguishable
//  from a muted failure (AGENTS.md: never weaken a guardrail to make a change
//  pass), and this file is where that distinction has to live.
// =============================================================================

/**
 * Files Nuxt loads by CONVENTION rather than by import.
 *
 * Nothing imports a page, a layout, a middleware or a `navigation.ts` - the
 * framework finds them by their path. To knip they are unreachable, so each
 * convention has to be declared as an entry point once.
 *
 * This list is also, usefully, the complete answer to "what does a frontend
 * module directory mean?" - see .ai/platform/docs/frontend-conventions.md.
 */
const nuxtConventions = (root: string) => [
  `${root}/app.vue`,
  `${root}/{pages,layouts,middleware,plugins,composables,components,stores,utils}/**/*.{ts,vue}`,
  `${root}/types/**/*.d.ts`,
]

/** The same, for one module layer inside an app. */
const moduleConventions = (root: string) => [
  `${root}/modules/*/nuxt.config.ts`,
  `${root}/modules/*/{pages,layouts,middleware,plugins,composables,components,stores}/**/*.{ts,vue}`,
  // Contributed to the shell by the layer: a menu entry and a settings tab.
  // Found by `import.meta.glob` in the shell and in plugins/injections.client.ts,
  // which is an import knip cannot follow.
  `${root}/modules/*/{navigation,settings-tabs,injections}.ts`,
  `${root}/modules/*/i18n/locales/*.ts`,
]

/**
 * Catalogues are re-exported through a `.ts` wrapper rather than imported as
 * JSON, because Vite's json plugin wraps the i18n transform's output in
 * `JSON.parse` and the page dies on the server with a parse error.
 * See [[a-json-import-is-not-always-json]] - the wrappers exist for that reason
 * and are referenced from `nuxt.config.ts`, not imported.
 */
const i18nWrappers = ['i18n/locales/*.ts']

const app = (name: string) => ({
  entry: ['nuxt.config.ts', ...nuxtConventions('app'), ...moduleConventions('app'), ...i18nWrappers],
  project: ['**/*.{ts,vue}'],
  // Every one of these reaches the app through Nuxt's module system or its
  // auto-imports, so no source file names them and knip cannot see the edge.
  ignoreDependencies: ['vue', 'vue-router', 'nuxt'],
})

const config: KnipConfig = {
  workspaces: {
    '.': {
      // `make` is the entry point, not npm. The scripts exist so tooling that
      // expects npm scripts finds something sensible.
      ignoreBinaries: ['make'],
      // Run by `nuxt typecheck` inside each app, which knip cannot follow: the
      // binary is invoked by Nuxt, not named in any script here.
      //
      // esbuild is imported by nothing here. It is declared so the hoisted copy
      // is the one vite accepts (peer `^0.27 || ^0.28`): @intlify/bundle-utils
      // depends on `^0.25`, and with nothing else asking for esbuild at the
      // root, npm hoists that one and refuses vite's peer (ERESOLVE). Declared,
      // 0.28 sits at the root and 0.25 nests under bundle-utils. Remove it once
      // bundle-utils moves past 0.25.
      ignoreDependencies: ['vue-tsc', 'esbuild'],
    },
    'ui-kit': {
      entry: ['nuxt.config.ts', ...nuxtConventions('app'), ...i18nWrappers, 'types/**/*.d.ts'],
      project: ['**/*.{ts,vue}'],
      // Registered in `modules:` / `vite:` inside nuxt.config.ts, or auto-imported.
      ignoreDependencies: ['vue', 'vue-router', 'nuxt', '@nuxtjs/i18n', '@pinia/nuxt', 'tailwindcss', '@vueuse/core'],
    },
    frontend: app('frontend'),
    manager: app('manager'),
    landing: {
      entry: [
        'nuxt.config.ts',
        // @nuxt/content reads this by name; nothing imports it.
        'content.config.ts',
        // Nitro routes - robots.txt, sitemap.xml, llms.txt. Nothing imports them.
        'server/**/*.ts',
        ...nuxtConventions('app'),
        ...i18nWrappers,
      ],
      project: ['**/*.{ts,vue}'],
      ignoreDependencies: ['vue', 'vue-router', 'nuxt'],
    },
    e2e: {
      entry: ['playwright.config.ts', 'tests/**/*.spec.ts', 'support/**/*.ts'],
      project: ['**/*.ts'],
    },
  },
  // Generated from backend/openapi.json by `make types`; it is an artefact, not
  // source, and half of it is legitimately unused until an endpoint is called.
  ignore: ['ui-kit/types/api.d.ts'],
  /**
   * A Nuxt layer is `extends`-ed by DIRECTORY, and a directory is not a module
   * specifier. Nuxt resolves it against the layer's own `nuxt.config.ts`; knip
   * reads the same line as a broken import. The pattern is narrow on purpose -
   * a genuinely broken import anywhere else still fails.
   */
  ignoreUnresolved: [/^\.\/app\/modules\//],
  ignoreExportsUsedInFile: true,
}

export default config
