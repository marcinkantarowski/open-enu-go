import { useAuthStore } from '@ui-kit/stores/auth'

/**
 * Every route requires a session unless it says otherwise.
 *
 * Global and fail-closed, deliberately: a named `auth` middleware has to be
 * REMEMBERED on every page, and the page where it is forgotten is the one that
 * leaks. Opting out is the rarer
 * case, it is visible in the page itself, and forgetting it produces a redirect
 * rather than an exposure.
 *
 *     definePageMeta({ public: true })
 */
export default defineNuxtRouteMiddleware((to) => {
  if (to.meta.public === true) return

  const auth = useAuthStore()

  // `ready` is set by the session plugin once the refresh cookie has had its
  // one chance. Without waiting for it, "not signed in" and "not asked yet" are
  // the same state, and every reload bounces to the login page.
  if (!auth.ready || auth.isAuthenticated) return

  return navigateTo({
    path: '/login',
    // So that signing in returns to where the person was going, rather than
    // dropping them on the dashboard to navigate again.
    query: to.fullPath === '/' ? undefined : { next: to.fullPath },
  })
})
