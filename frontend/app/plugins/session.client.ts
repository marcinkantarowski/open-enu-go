import { useAuthStore } from '@ui-kit/stores/auth'
import { useUiStore } from '@ui-kit/stores/ui'

/**
 * Turn the refresh cookie back into a session, once, before the first route
 * guard runs.
 *
 * The access token lives in memory (ADR-0018), so every reload starts with
 * none. Without this the router would bounce a signed-in person to the login
 * page on every refresh - the cost of that design is exactly one round trip,
 * paid here.
 *
 * `parallel: false` and awaiting matter: a guard that runs before `ready` is
 * set cannot tell "not signed in" from "not asked yet".
 */
export default defineNuxtPlugin({
  name: 'open-enu:session',
  parallel: false,

  async setup() {
    const auth = useAuthStore()
    useUiStore().restore()

    if (auth.ready) return

    await useAuth().restore()
  },
})
