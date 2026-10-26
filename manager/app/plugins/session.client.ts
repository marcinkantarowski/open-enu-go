import { useAuthStore } from '@ui-kit/stores/auth'
import { useUiStore } from '@ui-kit/stores/ui'

/**
 * There is nothing to restore.
 *
 * Operator sessions have no refresh cookie (.ai/platform/docs/auth.md), so a
 * reload genuinely is a sign-out. This plugin exists only to mark the store
 * ready, because the route guard waits for that flag - without it, every
 * navigation would stall behind a restore that is never coming.
 */
export default defineNuxtPlugin({
  name: 'open-enu:operator-session',
  parallel: false,

  setup() {
    useUiStore().restore()

    const auth = useAuthStore()
    auth.ready = true
  },
})
