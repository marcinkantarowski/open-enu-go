import { useAuthStore } from '@ui-kit/stores/auth'

/** Fail-closed, as in the tenant app: a page opts out, never opts in. */
export default defineNuxtRouteMiddleware((to) => {
  if (to.meta.public === true) return

  const auth = useAuthStore()

  if (!auth.ready || auth.isAuthenticated) return

  return navigateTo('/login')
})
