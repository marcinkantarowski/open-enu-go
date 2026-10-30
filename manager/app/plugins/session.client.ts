import { useAuthStore } from '@ui-kit/stores/auth'
import { useUiStore } from '@ui-kit/stores/ui'

/**
 * There is nothing to restore.
 *
 * Operator sessions have no refresh cookie (.ai/platform/docs/auth.md), so a
 * reload genuinely is a sign-out. This plugin exists only to mark the store
 * ready, because the route guard waits for that flag - without it, every
 * navigation would stall behind a restore that is never coming.
 *
 * A session that ends on a page goes to the login at once: the token ran out,
 * or the API refused it, and the route guard only runs on a navigation. Signing
 * out leaves the page first, so it is not taken for an expiry.
 */
export default defineNuxtPlugin({
  name: 'open-enu:operator-session',
  parallel: false,

  setup() {
    useUiStore().restore()

    const auth = useAuthStore()
    auth.ready = true

    const router = useRouter()

    watch(() => auth.token, (token, before) => {
      if (before && !token && router.currentRoute.value.meta.public !== true) {
        void router.push({ path: '/login', query: { expired: '1' } })
      }
    })
  },
})
