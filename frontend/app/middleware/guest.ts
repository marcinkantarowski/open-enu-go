import { useAuthStore } from '@ui-kit/stores/auth'

/**
 * For the pages that only make sense signed OUT - login, register, reset.
 *
 * Without it, a signed-in person following an old link to /login gets a form
 * that, on submit, rotates a perfectly good session.
 */
export default defineNuxtRouteMiddleware(() => {
  const auth = useAuthStore()

  if (auth.ready && auth.isAuthenticated) return navigateTo('/')
})
