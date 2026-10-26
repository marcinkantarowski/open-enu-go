/**
 * Runs every module's injection registrations once, at boot.
 *
 * `injections.ts` files are side-effect modules: importing them IS registering
 * them. The glob exists so that adding a widget to another module's page is a
 * new file inside your own module and nothing else
 * (.ai/platform/docs/frontend-conventions.md).
 */
const registrations = import.meta.glob('../modules/*/injections.ts', { eager: true })

export default defineNuxtPlugin(() => {
  // Referenced so the bundler cannot treat the glob as dead code, and so the
  // count is visible when something does not appear where it was expected.
  if (import.meta.dev && Object.keys(registrations).length > 0) {
    console.debug(`[frontend] ${Object.keys(registrations).length} module injection file(s) loaded`)
  }
})
